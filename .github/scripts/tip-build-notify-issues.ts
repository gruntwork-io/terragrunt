/**
 * Notifies referenced GitHub issues when a fix lands in the tip build.
 *
 * After a PR is merged to main and the tip build is uploaded, this script
 * finds issue-closing keywords in the PR body and comments on each
 * referenced issue with installation instructions for the tip build.
 *
 * Loaded by `github-script`, which run on Node 24 and
 * type-strip this file, so imports must carry explicit extensions and the
 * syntax must stay erasable.
 */

export type Core = {
  info(msg: string): void;
  warning(msg: string): void;
  setFailed(msg: string): void;
};

export type Context = {
  repo: { owner: string; repo: string };
  sha: string;
  runId: number;
};

export type AssociatedPR = {
  number: number;
  merged_at: string | null;
  body?: string | null;
};

export type ListPullRequestsAssociatedWithCommit = (params: {
  owner: string;
  repo: string;
  commit_sha: string;
}) => Promise<{ data: AssociatedPR[] }>;

export type GithubClient = {
  paginate<T>(
    method: unknown,
    opts: unknown,
    mapFn: (response: unknown) => T[],
  ): Promise<T[]>;
  paginate(
    method: unknown,
    opts: unknown,
  ): Promise<Array<{ body?: string | null }>>;
  rest: {
    repos: {
      listPullRequestsAssociatedWithCommit: ListPullRequestsAssociatedWithCommit;
      compareCommitsWithBasehead: unknown;
    };
    issues: {
      listComments: unknown;
      createComment(params: {
        owner: string;
        repo: string;
        issue_number: number;
        body: string;
      }): Promise<unknown>;
    };
  };
};

export type IssueRef = { owner: string; repo: string; number: number };

export default async function notifyTipBuildIssues({
  github,
  context,
  core,
}: {
  github: GithubClient;
  context: Context;
  core: Core;
}) {
  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const commitSha = context.sha;

  try {
    const mergedPRs = await findMergedPRs({ github, owner, repo, commitSha });
    if (mergedPRs.length === 0) {
      core.info('No merged PR found for this commit. Skipping issue notification.');
      return;
    }

    const issueNumbers = new Set<number>();
    const prNumbers: number[] = [];

    for (const pr of mergedPRs) {
      if (!pr.body) {
        continue;
      }

      prNumbers.push(pr.number);
      const refs = parseIssueReferences(pr.body, owner, repo);

      for (const ref of refs) {
        if (ref.owner !== owner || ref.repo !== repo) {
          core.warning(`Skipping cross-repo issue reference: ${ref.owner}/${ref.repo}#${ref.number}`);
          continue;
        }
        issueNumbers.add(ref.number);
      }
    }

    if (issueNumbers.size === 0) {
      core.info('No issue-closing keywords found in PR body. Skipping.');
      return;
    }

    core.info(`Found ${issueNumbers.size} issue(s) referenced by PR(s) #${prNumbers.join(', #')}.`);

    let commented = 0;
    for (const issueNumber of issueNumbers) {
      const alreadyCommented = await hasExistingComment({ github, owner, repo, issueNumber, commitSha });
      if (alreadyCommented) {
        core.info(`Issue #${issueNumber} already has a tip build comment for ${commitSha}. Skipping.`);
        continue;
      }

      const body = buildComment({ owner, repo, commitSha, prNumbers, runId: context.runId });

      try {
        await github.rest.issues.createComment({
          owner,
          repo,
          issue_number: issueNumber,
          body,
        });
        core.info(`Commented on issue #${issueNumber}.`);
        commented++;
      } catch (err) {
        core.warning(`Failed to comment on issue #${issueNumber}: ${err instanceof Error ? err.message : String(err)}`);
      }
    }

    core.info(`Notified ${commented} issue(s) about tip build.`);
  } catch (error) {
    core.setFailed(`Failed to notify issues: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/**
 * Finds merged PRs associated with the given commit SHA.
 */
export async function findMergedPRs({
  github,
  owner,
  repo,
  commitSha,
}: {
  github: {
    rest: {
      repos: {
        listPullRequestsAssociatedWithCommit: ListPullRequestsAssociatedWithCommit;
      };
    };
  };
  owner: string;
  repo: string;
  commitSha: string;
}): Promise<AssociatedPR[]> {
  const { data: pullRequests } =
    await github.rest.repos.listPullRequestsAssociatedWithCommit({
      owner,
      repo,
      commit_sha: commitSha,
    });

  return pullRequests.filter((pr) => pr.merged_at !== null);
}

/**
 * Parses issue-closing keywords from a PR body.
 *
 * Matches patterns like:
 *   closes #123, fixes #456, resolves #789
 *   close #123, fix #456, resolve #789
 *   closed #123, fixed #456, resolved #789
 *   fixes #123, #456, #789
 *   closes https://github.com/owner/repo/issues/123
 */
export function parseIssueReferences(
  body: string,
  defaultOwner: string,
  defaultRepo: string,
): IssueRef[] {
  const pattern = /\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+((?:(?:https?:\/\/github\.com\/[^/\s]+\/[^/\s]+\/issues\/\d+|#\d+)[\s,&]*(?:and\s+)?)+)/gi;
  const refs: IssueRef[] = [];
  const seen = new Set<string>();

  let match;
  while ((match = pattern.exec(body)) !== null) {
    const refsStr = match[1];

    const urlRefPattern = /https?:\/\/github\.com\/([^/]+)\/([^/]+)\/issues\/(\d+)/g;
    let urlMatch;
    while ((urlMatch = urlRefPattern.exec(refsStr)) !== null) {
      addRef(urlMatch[1], urlMatch[2], parseInt(urlMatch[3], 10));
    }

    const shortRefPattern = /#(\d+)/g;
    let shortMatch;
    while ((shortMatch = shortRefPattern.exec(refsStr)) !== null) {
      addRef(defaultOwner, defaultRepo, parseInt(shortMatch[1], 10));
    }
  }

  function addRef(refOwner: string, refRepo: string, refNumber: number) {
    const key = `${refOwner}/${refRepo}#${refNumber}`;
    if (!seen.has(key)) {
      seen.add(key);
      refs.push({ owner: refOwner, repo: refRepo, number: refNumber });
    }
  }

  return refs;
}

/**
 * Checks whether a tip build comment for this commit already exists on the issue.
 */
export async function hasExistingComment({
  github,
  owner,
  repo,
  issueNumber,
  commitSha,
}: {
  github: {
    paginate(
      method: unknown,
      opts: unknown,
    ): Promise<Array<{ body?: string | null }>>;
    rest: { issues: { listComments: unknown } };
  };
  owner: string;
  repo: string;
  issueNumber: number;
  commitSha: string;
}): Promise<boolean> {
  const comments = await github.paginate(github.rest.issues.listComments, {
    owner,
    repo,
    issue_number: issueNumber,
    per_page: 100,
  });

  return comments.some((c) => c.body && c.body.includes(`\`tip-${commitSha}\``));
}

/**
 * Builds the comment body to post on the issue.
 */
export function buildComment({
  owner,
  repo,
  commitSha,
  prNumbers,
  runId,
}: {
  owner: string;
  repo: string;
  commitSha: string;
  prNumbers: number[];
  runId: number;
}): string {
  const prLinks = prNumbers.map((n) => `#${n}`).join(', ');
  const runUrl = `https://github.com/${owner}/${repo}/actions/runs/${runId}`;

  return [
    `A fix for this issue has been included in the latest [\`tip\` build](${runUrl}) (\`tip-${commitSha}\`), built from ${prLinks}.`,
    '',
    `You can install the tip build by following [these instructions](https://docs.terragrunt.com/getting-started/install/#install-tip-or-test-builds).`,
    '',
    'This fix will also be included in a future release.',
  ].join('\n');
}
