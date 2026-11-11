/**
 * Opens a chained pull request against each open PR whose docs still target a
 * version that has shipped.
 *
 * A PR written before a release puts its changelog entries in that release's
 * directory and gates its docs with <Before>/<Since> for that version. If the
 * release ships first, the PR has to move to the next patch release. For every
 * recently updated open PR, this script:
 *
 *   - moves changelog entries the PR adds under a released version's directory
 *     to the next patch release's directory and rewrites their `version`
 *     frontmatter, and
 *   - bumps the version of every <Before>/<Since> gate the PR adds for a
 *     released version.
 *
 * The result is one commit on a `docs/retarget-pr-<number>` branch, proposed as
 * a PR into the original PR's branch. PRs from forks are skipped because the
 * workflow token cannot push to them.
 *
 * Loaded by `github-script`, which runs on Node 24 and type-strips this file,
 * so imports must carry explicit extensions and the syntax must stay erasable.
 */
import { isReleased } from "./docs-cleanup-version-gates.ts";
import type { Context, Core } from "./tip-build-notify-issues.ts";

const BRANCH_PREFIX = "docs/retarget-pr-";
const DEFAULT_MAX_AGE_DAYS = 30;
const MS_PER_DAY = 24 * 60 * 60 * 1000;

const DOCS_FILE = /^docs\/src\/.+\.mdx$/;
const CHANGELOG_ENTRY = /^(docs\/src\/data\/changelog\/)([^/]+)(\/.+\.mdx)$/;
const GATE_OPENING = /<(Before|Since)(\s+version=["'])([^"']+)(["']\s*>)/g;
const FRONTMATTER = /^---\n[\s\S]*?\n---/;
const FRONTMATTER_VERSION = /^(version:\s*["']?)([^"'\s]+)(["']?\s*)$/m;

export type PullRequest = {
  number: number;
  updated_at: string;
  head: {
    ref: string;
    sha: string;
    repo: { full_name: string } | null;
  };
};

export type PullRequestFile = {
  filename: string;
  status: string;
  changes: number;
  patch?: string;
};

export type FileChanges = {
  additions: Array<{ path: string; contents: string }>;
  deletions: Array<{ path: string }>;
};

type RepoParams = { owner: string; repo: string };

export type RetargetClient = {
  paginate<T>(method: unknown, opts: unknown): Promise<T[]>;
  graphql(query: string, variables: Record<string, unknown>): Promise<unknown>;
  rest: {
    pulls: {
      list: unknown;
      listFiles: unknown;
      create(
        params: RepoParams & {
          title: string;
          head: string;
          base: string;
          body: string;
        },
      ): Promise<{ data: { number: number } }>;
    };
    repos: {
      getContent(
        params: RepoParams & { path: string; ref: string },
      ): Promise<{ data: unknown }>;
    };
    git: {
      createRef(
        params: RepoParams & { ref: string; sha: string },
      ): Promise<unknown>;
      updateRef(
        params: RepoParams & { ref: string; sha: string; force: boolean },
      ): Promise<unknown>;
    };
    issues: {
      addLabels(
        params: RepoParams & { issue_number: number; labels: string[] },
      ): Promise<unknown>;
    };
  };
};

export default async function retargetOpenPRs({
  github,
  context,
  core,
  tagName,
  maxAgeDays = DEFAULT_MAX_AGE_DAYS,
  now = new Date(),
}: {
  github: RetargetClient;
  context: Pick<Context, "repo">;
  core: Core;
  tagName: string | undefined;
  maxAgeDays?: number;
  now?: Date;
}) {
  const { owner, repo } = context.repo;

  if (!tagName) {
    core.setFailed("tagName is required.");
    return;
  }
  if (!Number.isFinite(maxAgeDays) || maxAgeDays <= 0) {
    core.setFailed(`maxAgeDays must be a positive number, got ${maxAgeDays}.`);
    return;
  }

  let nextTag: string;
  try {
    nextTag = nextPatchVersion(tagName);
  } catch (error) {
    core.setFailed(errorMessage(error));
    return;
  }

  let prs: PullRequest[];
  try {
    prs = await github.paginate<PullRequest>(github.rest.pulls.list, {
      owner,
      repo,
      state: "open",
      sort: "updated",
      direction: "desc",
      per_page: 100,
    });
  } catch (error) {
    core.setFailed(`Failed to list open PRs: ${errorMessage(error)}`);
    return;
  }

  const cutoff = now.getTime() - maxAgeDays * MS_PER_DAY;
  let opened = 0;
  let failed = 0;

  for (const pr of prs) {
    const skip = skipReason({ pr, owner, repo, cutoff, maxAgeDays });
    if (skip) {
      core.info(`Skipping #${pr.number}: ${skip}.`);
      continue;
    }

    try {
      const retargeted = await retargetPR({
        github,
        core,
        owner,
        repo,
        pr,
        tagName,
        nextTag,
      });
      if (retargeted) {
        opened++;
      }
    } catch (error) {
      failed++;
      core.warning(`Failed to re-target #${pr.number}: ${errorMessage(error)}`);
    }
  }

  core.info(`Re-targeted ${opened} PR(s) to ${nextTag}.`);
  if (failed > 0) {
    core.setFailed(`Failed to re-target ${failed} PR(s).`);
  }
}

/**
 * Returns the patch release after `tag`, keeping its leading "v" if it has one.
 *
 * Throws when `tag` is not a plain X.Y.Z version.
 */
export function nextPatchVersion(tag: string): string {
  const match = /^(v?)(\d+)\.(\d+)\.(\d+)$/.exec(tag);
  if (!match) {
    throw new Error(`Cannot compute the next patch release of "${tag}".`);
  }
  const [, prefix, major, minor, patch] = match;
  return `${prefix}${major}.${minor}.${Number(patch) + 1}`;
}

/**
 * Returns the 1-indexed line numbers, in the new version of a file, of the
 * lines a unified diff `patch` adds.
 */
export function addedLineNumbers(patch: string): Set<number> {
  const added = new Set<number>();
  let line = 0;
  for (const row of patch.split("\n")) {
    const hunk = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(row);
    if (hunk) {
      line = Number(hunk[1]);
      continue;
    }
    if (row.startsWith("+")) {
      added.add(line);
      line++;
      continue;
    }
    if (row.startsWith(" ")) {
      line++;
    }
  }
  return added;
}

/**
 * Bumps released <Before>/<Since> gates to `nextTag` on the lines `isAdded`
 * accepts. Gates on other lines belong to the base branch and stay as they
 * are.
 */
export function retargetGates({
  source,
  isAdded,
  latestTag,
  nextTag,
}: {
  source: string;
  isAdded: (lineNumber: number) => boolean;
  latestTag: string;
  nextTag: string;
}): string {
  return source
    .split("\n")
    .map((line, index) => {
      if (!isAdded(index + 1)) {
        return line;
      }
      return line.replace(
        GATE_OPENING,
        (match, tag: string, open: string, version: string, close: string) => {
          if (!isReleased(latestTag, version)) {
            return match;
          }
          return `<${tag}${open}${matchPrefix(nextTag, version)}${close}`;
        },
      );
    })
    .join("\n");
}

/**
 * Rewrites a released `version` in a changelog entry's frontmatter to
 * `nextTag`.
 */
export function retargetFrontmatter({
  source,
  latestTag,
  nextTag,
}: {
  source: string;
  latestTag: string;
  nextTag: string;
}): string {
  return source.replace(FRONTMATTER, (frontmatter) =>
    frontmatter.replace(
      FRONTMATTER_VERSION,
      (match, open: string, version: string, close: string) => {
        if (!isReleased(latestTag, version)) {
          return match;
        }
        return `${open}${matchPrefix(nextTag, version)}${close}`;
      },
    ),
  );
}

/**
 * Computes the file changes that move one PR's docs from released versions to
 * `nextTag`. `readFile` returns a file's content at the PR's head commit.
 */
export async function planRetarget({
  files,
  latestTag,
  nextTag,
  readFile,
  warn,
}: {
  files: PullRequestFile[];
  latestTag: string;
  nextTag: string;
  readFile: (path: string) => Promise<string>;
  warn: (message: string) => void;
}): Promise<FileChanges> {
  const touched = new Set(files.map((file) => file.filename));
  const plans = await Promise.all(
    files
      .filter(
        (file) => DOCS_FILE.test(file.filename) && file.status !== "removed",
      )
      .map((file) => {
        if (file.status === "added") {
          return planAddedFile({
            file,
            touched,
            latestTag,
            nextTag,
            readFile,
            warn,
          });
        }
        return planModifiedFile({ file, latestTag, nextTag, readFile, warn });
      }),
  );

  return {
    additions: plans.flatMap((plan) => plan.additions),
    deletions: plans.flatMap((plan) => plan.deletions),
  };
}

const NO_CHANGES: FileChanges = { additions: [], deletions: [] };

async function planModifiedFile({
  file,
  latestTag,
  nextTag,
  readFile,
  warn,
}: {
  file: PullRequestFile;
  latestTag: string;
  nextTag: string;
  readFile: (path: string) => Promise<string>;
  warn: (message: string) => void;
}): Promise<FileChanges> {
  if (file.patch === undefined) {
    if (file.changes > 0) {
      warn(
        `No diff available for ${file.filename}; its gates were not checked.`,
      );
    }
    return NO_CHANGES;
  }
  if (!file.patch.includes("<Before") && !file.patch.includes("<Since")) {
    return NO_CHANGES;
  }

  const added = addedLineNumbers(file.patch);
  const source = await readFile(file.filename);
  const contents = retargetGates({
    source,
    isAdded: (line) => added.has(line),
    latestTag,
    nextTag,
  });
  if (contents === source) {
    return NO_CHANGES;
  }
  return { additions: [{ path: file.filename, contents }], deletions: [] };
}

async function planAddedFile({
  file,
  touched,
  latestTag,
  nextTag,
  readFile,
  warn,
}: {
  file: PullRequestFile;
  touched: Set<string>;
  latestTag: string;
  nextTag: string;
  readFile: (path: string) => Promise<string>;
  warn: (message: string) => void;
}): Promise<FileChanges> {
  const source = await readFile(file.filename);
  const gated = retargetGates({
    source,
    isAdded: () => true,
    latestTag,
    nextTag,
  });

  const entry = CHANGELOG_ENTRY.exec(file.filename);
  if (!entry || !isReleased(latestTag, entry[2])) {
    if (gated === source) {
      return NO_CHANGES;
    }
    return {
      additions: [{ path: file.filename, contents: gated }],
      deletions: [],
    };
  }

  const path = `${entry[1]}${matchPrefix(nextTag, entry[2])}${entry[3]}`;
  if (touched.has(path)) {
    warn(`Not moving ${file.filename}: ${path} is already part of the PR.`);
    return NO_CHANGES;
  }
  return {
    additions: [
      {
        path,
        contents: retargetFrontmatter({ source: gated, latestTag, nextTag }),
      },
    ],
    deletions: [{ path: file.filename }],
  };
}

export function buildBody({
  prNumber,
  tagName,
  nextTag,
  changes,
}: {
  prNumber: number;
  tagName: string;
  nextTag: string;
  changes: FileChanges;
}): string {
  const moved = new Set(changes.deletions.map((deletion) => deletion.path));
  const lines = [
    `\`${tagName}\` shipped before #${prNumber} merged, so this re-targets its docs to \`${nextTag}\`:`,
    "",
    `- Changelog entries #${prNumber} adds for a released version move to \`${nextTag}\`.`,
    `- \`<Before>\` and \`<Since>\` gates #${prNumber} adds for a released version now name \`${nextTag}\`.`,
    "",
    "Files changed:",
    "",
    ...changes.additions.map((addition) => `- \`${addition.path}\``),
    "",
    `${moved.size} changelog entr${moved.size === 1 ? "y" : "ies"} moved. Close this PR if #${prNumber} targets a different release.`,
    "",
    "Generated by the `docs-cleanup-version-gates` workflow.",
  ];
  return lines.join("\n");
}

function skipReason({
  pr,
  owner,
  repo,
  cutoff,
  maxAgeDays,
}: {
  pr: PullRequest;
  owner: string;
  repo: string;
  cutoff: number;
  maxAgeDays: number;
}): string | undefined {
  if (new Date(pr.updated_at).getTime() < cutoff) {
    return `not updated in the last ${maxAgeDays} day(s)`;
  }
  if (pr.head.repo?.full_name !== `${owner}/${repo}`) {
    return "its branch is in a fork";
  }
  if (pr.head.ref.startsWith(BRANCH_PREFIX)) {
    return "it is a re-target PR";
  }
  return undefined;
}

async function retargetPR({
  github,
  core,
  owner,
  repo,
  pr,
  tagName,
  nextTag,
}: {
  github: RetargetClient;
  core: Core;
  owner: string;
  repo: string;
  pr: PullRequest;
  tagName: string;
  nextTag: string;
}): Promise<boolean> {
  const files = await github.paginate<PullRequestFile>(
    github.rest.pulls.listFiles,
    { owner, repo, pull_number: pr.number, per_page: 100 },
  );

  const changes = await planRetarget({
    files,
    latestTag: tagName,
    nextTag,
    readFile: (path) =>
      readFileAt({ github, owner, repo, path, ref: pr.head.sha }),
    warn: (message) => core.warning(`#${pr.number}: ${message}`),
  });
  if (changes.additions.length === 0) {
    return false;
  }

  const branch = `${BRANCH_PREFIX}${pr.number}`;
  await resetBranch({ github, owner, repo, branch, sha: pr.head.sha });

  const title = `docs: Re-targeting #${pr.number} to \`${nextTag}\``;
  // createCommitOnBranch signs the commit, which the REST git endpoints do not.
  await github.graphql(
    `mutation ($input: CreateCommitOnBranchInput!) {
      createCommitOnBranch(input: $input) { commit { oid } }
    }`,
    {
      input: {
        branch: { repositoryNameWithOwner: `${owner}/${repo}`, branchName: branch },
        expectedHeadOid: pr.head.sha,
        message: { headline: title },
        fileChanges: {
          additions: changes.additions.map(({ path, contents }) => ({
            path,
            contents: Buffer.from(contents, "utf8").toString("base64"),
          })),
          deletions: changes.deletions,
        },
      },
    },
  );

  const existing = await github.paginate<{ number: number }>(
    github.rest.pulls.list,
    { owner, repo, state: "open", head: `${owner}:${branch}`, per_page: 100 },
  );
  if (existing.length > 0) {
    core.info(`Refreshed #${existing[0].number} for #${pr.number}.`);
    return true;
  }

  const { data: created } = await github.rest.pulls.create({
    owner,
    repo,
    title,
    head: branch,
    base: pr.head.ref,
    body: buildBody({ prNumber: pr.number, tagName, nextTag, changes }),
  });
  await github.rest.issues.addLabels({
    owner,
    repo,
    issue_number: created.number,
    labels: ["automated"],
  });
  core.info(`Opened #${created.number} to re-target #${pr.number}.`);
  return true;
}

async function readFileAt({
  github,
  owner,
  repo,
  path,
  ref,
}: {
  github: RetargetClient;
  owner: string;
  repo: string;
  path: string;
  ref: string;
}): Promise<string> {
  const { data } = await github.rest.repos.getContent({ owner, repo, path, ref });
  const file = data as { content?: string; encoding?: string };
  if (typeof file.content !== "string" || file.encoding !== "base64") {
    throw new Error(`Cannot read ${path} at ${ref}.`);
  }
  return Buffer.from(file.content, "base64").toString("utf8");
}

/**
 * Points `branch` at `sha`, creating it if needed, so a rerun starts from the
 * PR's current head.
 */
async function resetBranch({
  github,
  owner,
  repo,
  branch,
  sha,
}: {
  github: RetargetClient;
  owner: string;
  repo: string;
  branch: string;
  sha: string;
}): Promise<void> {
  try {
    await github.rest.git.createRef({
      owner,
      repo,
      ref: `refs/heads/${branch}`,
      sha,
    });
  } catch (error) {
    // 422 is GitHub's answer when the ref already exists.
    if ((error as { status?: number }).status !== 422) {
      throw error;
    }
    await github.rest.git.updateRef({
      owner,
      repo,
      ref: `heads/${branch}`,
      sha,
      force: true,
    });
  }
}

function matchPrefix(nextTag: string, original: string): string {
  const bare = nextTag.replace(/^v/, "");
  return original.startsWith("v") ? `v${bare}` : bare;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export { BRANCH_PREFIX, DEFAULT_MAX_AGE_DAYS };
