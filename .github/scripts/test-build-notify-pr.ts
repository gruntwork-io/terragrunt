/**
 * Comments on a pull request with install instructions for its test build.
 *
 * Loaded by `github-script`, which run on Node 24 and
 * type-strip this file, so imports must carry explicit extensions and the
 * syntax must stay erasable.
 */

import type { Core } from "./tip-build-notify-issues.ts";

export type Context = {
  repo: { owner: string; repo: string };
  runId: number;
};

export type GithubClient = {
  paginate(
    method: unknown,
    opts: unknown,
  ): Promise<Array<{ body?: string | null }>>;
  rest: {
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

export default async function notifyTestBuildPR({
  github,
  context,
  core,
  prNumber,
  commitSha,
}: {
  github: GithubClient;
  context: Context;
  core: Core;
  prNumber: number;
  commitSha: string;
}) {
  const owner = context.repo.owner;
  const repo = context.repo.repo;

  try {
    const comments = await github.paginate(github.rest.issues.listComments, {
      owner,
      repo,
      issue_number: prNumber,
      per_page: 100,
    });

    if (comments.some((c) => c.body && c.body.includes(`\`test-${commitSha}\``))) {
      core.info(`PR #${prNumber} already has a test build comment for ${commitSha}. Skipping.`);
      return;
    }

    await github.rest.issues.createComment({
      owner,
      repo,
      issue_number: prNumber,
      body: buildComment({ owner, repo, commitSha, runId: context.runId }),
    });
    core.info(`Commented on PR #${prNumber}.`);
  } catch (error) {
    core.setFailed(`Failed to comment on PR #${prNumber}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/**
 * Builds the comment body to post on the pull request.
 */
export function buildComment({
  owner,
  repo,
  commitSha,
  runId,
}: {
  owner: string;
  repo: string;
  commitSha: string;
  runId: number;
}): string {
  const runUrl = `https://github.com/${owner}/${repo}/actions/runs/${runId}`;

  return [
    `A [\`test\` build](${runUrl}) of this pull request is available (\`test-${commitSha}\`).`,
    '',
    'Install it with:',
    '',
    '```bash',
    `curl -sSfL --proto '=https' --tlsv1.2 https://terragrunt.com/install | bash -s -- --test --commit ${commitSha}`,
    '```',
    '',
    'For manual installation and signature verification, see [these instructions](https://docs.terragrunt.com/getting-started/install/#install-tip-or-test-builds).',
  ].join('\n');
}
