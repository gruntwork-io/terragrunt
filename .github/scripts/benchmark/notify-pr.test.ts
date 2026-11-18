import { describe, expect, test } from "bun:test";

import notifyBenchmarkPR, {
  BOT_LOGIN,
  buildClearedComment,
  buildFlaggedComment,
  MARKER,
  type Comment,
} from "./notify-pr.ts";

const SHA = "0123456789abcdef0123456789abcdef01234567";
const context = { repo: { owner: "gruntwork-io", repo: "terragrunt" }, sha: SHA, runId: 99 };
const flagged = { regressions: 1, improvements: 0 };
const quiet = { regressions: 0, improvements: 0 };
const summary = "## Benchmark: push\n\n| `find.flat.10` | +20.0% |\n";

function fakeGithub({
  pulls = [{ number: 7, state: "open" }],
  comments = [],
  failWith,
}: {
  pulls?: Array<{ number: number; state: string }>;
  comments?: Comment[];
  failWith?: Error;
}) {
  const created: Array<{ issue_number: number; body: string }> = [];
  const updated: Array<{ comment_id: number; body: string }> = [];
  const mutations: Array<{ query: string; id: unknown }> = [];

  const github = {
    paginate: async (_method: unknown, _opts: unknown) => comments,
    graphql: async (query: string, variables: Record<string, unknown>) => {
      mutations.push({ query, id: variables.id });
    },
    rest: {
      repos: {
        listPullRequestsAssociatedWithCommit: async (_params: unknown) => {
          if (failWith) {
            throw failWith;
          }

          return { data: pulls };
        },
      },
      issues: {
        listComments: {},
        createComment: async (params: { issue_number: number; body: string }) => {
          created.push({ issue_number: params.issue_number, body: params.body });
        },
        updateComment: async (params: { comment_id: number; body: string }) => {
          updated.push({ comment_id: params.comment_id, body: params.body });
        },
      },
    },
  };

  return { github, created, updated, mutations };
}

function fakeCore() {
  const failures: string[] = [];
  const core = {
    info: (_msg: string) => {},
    warning: (_msg: string) => {},
    setFailed: (msg: string) => {
      failures.push(msg);
    },
  };

  return { core, failures };
}

const earlier: Comment = {
  id: 11,
  node_id: "IC_fake",
  body: `${MARKER}\n## Benchmark: push\nold`,
  user: { login: BOT_LOGIN },
};
const unrelated: Comment = { id: 12, node_id: "IC_other", body: "Looks good to me", user: { login: "reviewer" } };
const impostor: Comment = { id: 13, node_id: "IC_impostor", body: `${MARKER}\nnot the bot`, user: { login: "reviewer" } };

describe("notifyBenchmarkPR", () => {
  test("creates a comment when a metric is flagged and none exists", async () => {
    const { github, created, updated, mutations } = fakeGithub({ comments: [unrelated] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(failures).toEqual([]);
    expect(created).toHaveLength(1);
    expect(created[0].issue_number).toBe(7);
    expect(created[0].body).toStartWith(MARKER);
    expect(created[0].body).toContain("| `find.flat.10` | +20.0% |");
    expect(updated).toEqual([]);
    expect(mutations).toEqual([]);
  });

  test("does nothing when no metric is flagged and no comment exists", async () => {
    const { github, created, updated, mutations } = fakeGithub({ comments: [unrelated] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: quiet, summary });

    expect(failures).toEqual([]);
    expect(created).toEqual([]);
    expect(updated).toEqual([]);
    expect(mutations).toEqual([]);
  });

  test("updates and hides the earlier comment when no metric is flagged any more", async () => {
    const { github, created, updated, mutations } = fakeGithub({ comments: [unrelated, earlier] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: quiet, summary });

    expect(failures).toEqual([]);
    expect(created).toEqual([]);
    expect(updated).toHaveLength(1);
    expect(updated[0].comment_id).toBe(11);
    expect(updated[0].body).toContain("No metric regressed or improved");
    expect(mutations).toHaveLength(1);
    expect(mutations[0].query).toContain("minimizeComment");
    expect(mutations[0].query).toContain("RESOLVED");
    expect(mutations[0].id).toBe("IC_fake");
  });

  test("updates and unhides the earlier comment when a metric is flagged again", async () => {
    const { github, created, updated, mutations } = fakeGithub({ comments: [earlier] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(failures).toEqual([]);
    expect(created).toEqual([]);
    expect(updated).toHaveLength(1);
    expect(updated[0].body).toContain("| `find.flat.10` | +20.0% |");
    expect(mutations).toHaveLength(1);
    expect(mutations[0].query).toContain("unminimizeComment");
    expect(mutations[0].id).toBe("IC_fake");
  });

  test("ignores a comment with the marker that the bot did not write", async () => {
    const { github, created, updated, mutations } = fakeGithub({ comments: [impostor] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(failures).toEqual([]);
    expect(created).toHaveLength(1);
    expect(updated).toEqual([]);
    expect(mutations).toEqual([]);
  });

  test("counts an improvement as flagged", async () => {
    const { github, created } = fakeGithub({});
    const { core } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: { regressions: 0, improvements: 2 }, summary });

    expect(created).toHaveLength(1);
  });

  test("skips a commit with no open pull request", async () => {
    const { github, created, updated } = fakeGithub({ pulls: [{ number: 3, state: "closed" }] });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(failures).toEqual([]);
    expect(created).toEqual([]);
    expect(updated).toEqual([]);
  });

  test("comments on every open pull request for the commit", async () => {
    const { github, created } = fakeGithub({
      pulls: [
        { number: 7, state: "open" },
        { number: 8, state: "open" },
      ],
    });
    const { core } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(created.map((comment) => comment.issue_number)).toEqual([7, 8]);
  });

  test("fails the step when the API call fails", async () => {
    const { github } = fakeGithub({ failWith: new Error("boom") });
    const { core, failures } = fakeCore();

    await notifyBenchmarkPR({ github, context, core, report: flagged, summary });

    expect(failures).toEqual(["Failed to update the benchmark comment: boom"]);
  });
});

describe("buildFlaggedComment", () => {
  test("ends with the commit and a link to the run", () => {
    const body = buildFlaggedComment({ owner: "gruntwork-io", repo: "terragrunt", sha: SHA, runId: 99, summary });

    expect(body).toContain("Measured at `0123456789ab`.");
    expect(body).toContain("https://github.com/gruntwork-io/terragrunt/actions/runs/99");
  });

  test("cuts a summary that would not fit in a comment", () => {
    const body = buildFlaggedComment({
      owner: "gruntwork-io",
      repo: "terragrunt",
      sha: SHA,
      runId: 99,
      summary: "x".repeat(100_000),
    });

    expect(body.length).toBeLessThan(65_536);
    expect(body).toContain("The summary was cut short.");
    expect(body).toContain("actions/runs/99");
  });
});

describe("buildClearedComment", () => {
  test("carries the marker so a later push finds it", () => {
    const body = buildClearedComment({ owner: "gruntwork-io", repo: "terragrunt", sha: SHA, runId: 99 });

    expect(body).toStartWith(MARKER);
    expect(body).toContain("Measured at `0123456789ab`.");
  });
});
