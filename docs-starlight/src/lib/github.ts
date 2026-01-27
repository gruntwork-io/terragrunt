const CACHE_TTL_MS = 60 * 60 * 1000; // 1 hour in milliseconds

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

const cache = new Map<string, CacheEntry<unknown>>();

/**
 * Memoized fetch that caches results for 1 hour.
 * Used to avoid rate limiting on GitHub API calls during builds.
 */
async function memoizedFetch<T>(
  cacheKey: string,
  fetchFn: () => Promise<T>
): Promise<T> {
  const now = Date.now();
  const cached = cache.get(cacheKey) as CacheEntry<T> | undefined;

  if (cached && now - cached.timestamp < CACHE_TTL_MS) {
    return cached.data;
  }

  const data = await fetchFn();
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

  try {
    return await memoizedFetch(cacheKey, async () => {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}`,
        {
          headers: {
            'User-Agent': 'Terragrunt-Docs',
          },
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

      return response.json();
    });
  } catch (error) {
    console.error(`Error fetching GitHub repo ${owner}/${repo}:`, error);
    return null;
  }
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

  try {
    return await memoizedFetch(cacheKey, async () => {
      const response = await fetch(
        `https://api.github.com/repos/${owner}/${repo}/releases/latest`,
        {
          headers: {
            'User-Agent': 'Terragrunt-Docs',
          },
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

      return response.json();
    });
  } catch (error) {
    console.error(`Error fetching latest release for ${owner}/${repo}:`, error);
    return null;
  }
}

/**
 * Formats a star count for display (e.g., 8600 -> "8.6k")
 */
export function formatStarCount(stars: number): string {
  return (stars / 1000).toFixed(1) + 'k';
}
