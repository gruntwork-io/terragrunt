/**
 * Prints the GitHub release notes for a version that has no release yet.
 *
 * The notes are the Markdown the docs changelog page offers under "Copy to
 * GitHub Releases": the changelog entries of the version, then the pull
 * requests merged since the latest release.
 *
 * A pre-release takes the changelog entries of the version it leads to, so
 * `v1.3.0-rc1` prints the entries of `v1.3.0`.
 *
 * Usage: HEAD_SHA=<commit> bun run release-notes <version>
 * Set GITHUB_TOKEN to avoid the unauthenticated GitHub API rate limit.
 */
import path from "node:path";
import {
  entriesForVersion,
  pullRequestsFromCommits,
  releaseNotesMarkdown,
} from "../src/lib/changelog";
import { readChangelogEntries } from "../src/lib/changelog-files";
import { compareCommits, getLatestRelease, type GitHubCommit } from "../src/lib/github";

const OWNER = "gruntwork-io";
const REPO = "terragrunt";
const SITE_URL = "https://docs.terragrunt.com";

const VERSION = /^(v\d+\.\d+\.\d+)(?:[-+][0-9A-Za-z.+-]+)?$/;

export type GitHub = {
  latestReleaseTag(): Promise<string | null>;
  commitsBetween(base: string, head: string): Promise<GitHubCommit[] | null>;
};

/**
 * Returns the `vX.Y.Z` version whose changelog entries describe `version`,
 * which is `version` without its pre-release and build suffixes.
 */
export function changelogVersion(version: string): string {
  const match = VERSION.exec(version);
  if (match === null) {
    throw new Error(`version "${version}" must have the form vX.Y.Z[-pre-release]`);
  }
  return match[1];
}

/**
 * Returns the release notes for `version`, listing the pull requests between
 * the latest release and `head`.
 */
export async function buildReleaseNotes(
  version: string,
  head: string,
  changelogDir: string,
  github: GitHub,
): Promise<string> {
  const base = changelogVersion(version);
  const entries = entriesForVersion(readChangelogEntries(changelogDir), base);
  if (entries.length === 0) {
    throw new Error(`no changelog entries for ${base} under ${changelogDir}`);
  }

  const latestTag = await github.latestReleaseTag();
  if (latestTag === null) {
    throw new Error(`could not resolve the latest release of ${OWNER}/${REPO}`);
  }

  const commits = await github.commitsBetween(latestTag, head);
  if (commits === null) {
    throw new Error(`could not list the commits between ${latestTag} and ${head}`);
  }

  return releaseNotesMarkdown(entries, pullRequestsFromCommits(commits, OWNER, REPO), SITE_URL);
}

if (import.meta.main) {
  const version = process.argv[2];
  const head = process.env.HEAD_SHA;
  if (!version || !head) {
    console.error("Usage: HEAD_SHA=<commit> bun run release-notes <version>");
    process.exit(1);
  }

  const changelogDir = path.join(import.meta.dir, "..", "src", "data", "changelog");
  const notes = await buildReleaseNotes(version, head, changelogDir, {
    latestReleaseTag: async () => (await getLatestRelease(OWNER, REPO))?.tag_name ?? null,
    commitsBetween: (base, compareHead) => compareCommits(OWNER, REPO, base, compareHead),
  });
  console.log(notes);
}
