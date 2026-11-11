const CACHE_TTL_MS = 60 * 60 * 1000; // 1 hour in milliseconds
const FETCH_TIMEOUT_MS = 10 * 1000; // 10 second timeout for API requests

interface CacheEntry<T> {
  data: Promise<T>;
  timestamp: number;
}

const CACHE_KEY = Symbol.for('terragrunt-docs.github-cache');
const cache: Map<string, CacheEntry<unknown>> = ((
  globalThis as { [CACHE_KEY]?: Map<string, CacheEntry<unknown>> }
)[CACHE_KEY] ??= new Map());

/**
 * Returns the headers every GitHub API request sends. A `GITHUB_TOKEN` in the
 * environment is sent as a bearer token: without one, requests draw on a
 * per-address quota that a shared network exhausts, and a rate-limited build
 * treats every gated version as unreleased.
 */
function githubHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'User-Agent': 'Terragrunt-Docs' };

  const token = process.env.GITHUB_TOKEN;
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  return headers;
}

/**
 * Returns the result of `fetchFn` for `cacheKey`, calling it at most once an
 * hour. Every caller in the process shares that call and its result, failures
 * included, even when Astro loads this module more than once, as it does for
 * content sync and for prerendering. A build therefore sends one request per
 * key, and every page sees the same answer. `fetchFn` must not reject.
 */
function memoizedFetch<T>(
  cacheKey: string,
  fetchFn: () => Promise<T>
): Promise<T> {
  const now = Date.now();
  const cached = cache.get(cacheKey) as CacheEntry<T> | undefined;

  if (cached && now - cached.timestamp < CACHE_TTL_MS) {
    return cached.data;
  }

  const data = fetchFn();
  cache.set(cacheKey, { data, timestamp: now });
  return data;
}

interface GitHubRepoResponse {
  stargazers_count: number;
  [key: string]: unknown;
}

interface GitHubReleaseResponse {
  tag_name: string;
  name: string;
  html_url: string;
  published_at: string;
  body?: string;
  [key: string]: unknown;
}

export interface GitHubCommit {
  sha: string;
  html_url: string;
  commit: { message: string; author?: { name?: string } | null };
  // null when the committer isn't a linked GitHub user.
  author: { login: string } | null;
}

interface GitHubCompareResponse {
  commits: GitHubCommit[];
  [key: string]: unknown;
}

/**
 * Fetches GitHub repository data with memoization.
 * Results are cached for 1 hour to avoid rate limiting.
 */
export async function getGitHubRepo(
  owner: string,
  repo: string
): Promise<GitHubRepoResponse | null> {
  const cacheKey = `repo:${owner}/${repo}`;

  return memoizedFetch(cacheKey, async () => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

    try {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}`,
        {
          headers: githubHeaders(),
          signal: controller.signal,
        }
      );

      if (!response.ok) {
        console.error(
          `Failed to fetch GitHub repo ${owner}/${repo}:`,
          response.status,
          await response.text()
        );
        return null;
      }

      return await response.json();
    } catch (error) {
      console.error(`Error fetching GitHub repo ${owner}/${repo}:`, error);
      return null;
    } finally {
      clearTimeout(timeoutId);
    }
  });
}

/**
 * Fetches the latest release for a GitHub repository with memoization.
 * Results are cached for 1 hour to avoid rate limiting.
 */
export async function getLatestRelease(
  owner: string,
  repo: string
): Promise<GitHubReleaseResponse | null> {
  const cacheKey = `release:${owner}/${repo}`;

  return memoizedFetch(cacheKey, async () => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

    try {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}/releases/latest`,
        {
          headers: githubHeaders(),
          signal: controller.signal,
        }
      );

      if (!response.ok) {
        console.error(
          `Failed to fetch latest release for ${owner}/${repo}:`,
          response.status,
          await response.text()
        );
        return null;
      }

      return await response.json();
    } catch (error) {
      console.error(`Error fetching latest release for ${owner}/${repo}:`, error);
      return null;
    } finally {
      clearTimeout(timeoutId);
    }
  });
}

/**
 * Fetches a specific release by tag with memoization.
 * Results are cached for 1 hour to avoid rate limiting.
 */
export async function getReleaseByTag(
  owner: string,
  repo: string,
  tag: string
): Promise<GitHubReleaseResponse | null> {
  const cacheKey = `release-tag:${owner}/${repo}/${tag}`;

  return memoizedFetch(cacheKey, async () => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

    try {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}/releases/tags/${tag}`,
        {
          headers: githubHeaders(),
          signal: controller.signal,
        }
      );

      if (!response.ok) {
        if (response.status !== 404) {
          console.error(
            `Failed to fetch release ${tag} for ${owner}/${repo}:`,
            response.status,
            await response.text()
          );
        }
        return null;
      }

      return await response.json();
    } catch (error) {
      console.error(`Error fetching release ${tag} for ${owner}/${repo}:`, error);
      return null;
    } finally {
      clearTimeout(timeoutId);
    }
  });
}

/**
 * Compares two refs and returns the commits in `head` that are not in `base`,
 * memoized for 1 hour to avoid rate limiting. Used to list the pull requests
 * merged since the latest release for versions that aren't published yet.
 *
 * The compare endpoint returns at most 250 commits; that's sufficient here
 * because releases are cut frequently.
 */
export async function compareCommits(
  owner: string,
  repo: string,
  base: string,
  head: string
): Promise<GitHubCommit[] | null> {
  const cacheKey = `compare:${owner}/${repo}/${base}...${head}`;

  return memoizedFetch(cacheKey, async () => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);

    try {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}/compare/${base}...${head}`,
        {
          headers: githubHeaders(),
          signal: controller.signal,
        }
      );

      if (!response.ok) {
        if (response.status !== 404) {
          console.error(
            `Failed to compare ${base}...${head} for ${owner}/${repo}:`,
            response.status,
            await response.text()
          );
        }
        return null;
      }

      const data = (await response.json()) as GitHubCompareResponse;
      return data.commits;
    } catch (error) {
      console.error(
        `Error comparing ${base}...${head} for ${owner}/${repo}:`,
        error
      );
      return null;
    } finally {
      clearTimeout(timeoutId);
    }
  });
}

/**
 * Formats a star count for display (e.g., 8600 -> "8.6k")
 */
export function formatStarCount(stars: number): string {
  return (stars / 1000).toFixed(1) + 'k';
}
