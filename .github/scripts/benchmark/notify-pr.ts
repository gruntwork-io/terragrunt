/**
 * Keeps one benchmark comment on each open pull request for the pushed commit.
 *
 * A push that regresses or improves a metric creates the comment, or updates
 * and unhides the one already there. A push that changes nothing leaves a pull
 * request without a comment alone, and updates and hides a comment left by an
 * earlier push.
 *
 * Loaded by `github-script`, which runs on Node 24 and type-strips this file,
 * so imports must carry explicit extensions and the syntax must stay erasable.
 */

import type { Core } from "../tip-build-notify-issues.ts";

export type Context = {
  repo: { owner: string; repo: string };
  sha: string;
  runId: number;
};

export type Comment = { id: number; node_id: string; body?: string | null; user?: { login: string } | null };

export type GithubClient = {
  paginate(method: unknown, opts: unknown): Promise<Comment[]>;
  graphql(query: string, variables: Record<string, unknown>): Promise<unknown>;
  rest: {
    repos: {
      listPullRequestsAssociatedWithCommit(params: {
        owner: string;
        repo: string;
        commit_sha: string;
      }): Promise<{ data: Array<{ number: number; state: string }> }>;
    };
    issues: {
      listComments: unknown;
      createComment(params: { owner: string; repo: string; issue_number: number; body: string }): Promise<unknown>;
      updateComment(params: { owner: string; repo: string; comment_id: number; body: string }): Promise<unknown>;
    };
  };
};

export type ReportCounts = { regressions: number; improvements: number };

/** Identifies the benchmark comment among a pull request's comments. */
export const MARKER = "<!-- terragrunt-benchmark -->";

/** The account `GITHUB_TOKEN` comments as. Anyone can post the marker, so only its comments count. */
export const BOT_LOGIN = "github-actions[bot]";

/** GitHub rejects a comment body longer than 65,536 characters. */
const MAX_SUMMARY_LENGTH = 60_000;

/** A commit is normally the head of one open pull request, so more than this is not expected. */
const MAX_PULL_REQUESTS = 5;

const MINIMIZE = `mutation($id: ID!) {
  minimizeComment(input: { subjectId: $id, classifier: RESOLVED }) { clientMutationId }
}`;

const UNMINIMIZE = `mutation($id: ID!) {
  unminimizeComment(input: { subjectId: $id }) { clientMutationId }
}`;

export default async function notifyBenchmarkPR({
  github,
  context,
  core,
  report,
  summary,
}: {
  github: GithubClient;
  context: Context;
  core: Core;
  report: ReportCounts;
  summary: string;
}) {
  const { owner, repo } = context.repo;
  const flagged = report.regressions + report.improvements > 0;

  try {
    const { data: pulls } = await github.rest.repos.listPullRequestsAssociatedWithCommit({
      owner,
      repo,
      commit_sha: context.sha,
    });

    const open = pulls.filter((pull) => pull.state === "open").slice(0, MAX_PULL_REQUESTS);

    if (open.length === 0) {
      core.info(`No open pull request for ${context.sha}. Skipping benchmark comment.`);
      return;
    }

    for (const pull of open) {
      const comments = await github.paginate(github.rest.issues.listComments, {
        owner,
        repo,
        issue_number: pull.number,
        per_page: 100,
      });

      const existing = comments.find(
        (comment) => comment.user?.login === BOT_LOGIN && comment.body?.includes(MARKER),
      );

      if (!flagged && !existing) {
        core.info(`Nothing to report on PR #${pull.number}.`);
        continue;
      }

      const body = flagged
        ? buildFlaggedComment({ owner, repo, sha: context.sha, runId: context.runId, summary })
        : buildClearedComment({ owner, repo, sha: context.sha, runId: context.runId });

      if (!existing) {
        await github.rest.issues.createComment({ owner, repo, issue_number: pull.number, body });
        core.info(`Commented on PR #${pull.number}.`);
        continue;
      }

      await github.rest.issues.updateComment({ owner, repo, comment_id: existing.id, body });
      await github.graphql(flagged ? UNMINIMIZE : MINIMIZE, { id: existing.node_id });
      core.info(`Updated the comment on PR #${pull.number} and ${flagged ? "unhid" : "hid"} it.`);
    }
  } catch (error) {
    core.setFailed(`Failed to update the benchmark comment: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/** Builds the comment for a commit with regressed or improved metrics. */
export function buildFlaggedComment({
  owner,
  repo,
  sha,
  runId,
  summary,
}: {
  owner: string;
  repo: string;
  sha: string;
  runId: number;
  summary: string;
}): string {
  const shown =
    summary.length > MAX_SUMMARY_LENGTH
      ? `${summary.slice(0, MAX_SUMMARY_LENGTH)}\n\nThe summary was cut short. The run has all of it.\n`
      : summary;

  return [MARKER, shown.trimEnd(), "", footer({ owner, repo, sha, runId })].join("\n");
}

/** Builds the comment for a commit that cleared what an earlier commit was flagged for. */
export function buildClearedComment({
  owner,
  repo,
  sha,
  runId,
}: {
  owner: string;
  repo: string;
  sha: string;
  runId: number;
}): string {
  return [
    MARKER,
    "## Benchmark",
    "",
    "No metric regressed or improved against the base. An earlier commit on this pull request was flagged.",
    "",
    footer({ owner, repo, sha, runId }),
  ].join("\n");
}

function footer({ owner, repo, sha, runId }: { owner: string; repo: string; sha: string; runId: number }): string {
  return `Measured at \`${sha.slice(0, 12)}\`. [Full results and profiles](https://github.com/${owner}/${repo}/actions/runs/${runId}).`;
}
