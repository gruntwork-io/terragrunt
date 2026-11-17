import { describe, test, expect } from "bun:test";
import notifyTestBuildPR, { buildComment } from "./test-build-notify-pr";

const SHA = "0123456789abcdef0123456789abcdef01234567";

function fakeGithub(existing: Array<{ body?: string | null }>) {
  const created: Array<{ issue_number: number; body: string }> = [];
  const github = {
    paginate: async (_method: unknown, _opts: unknown) => existing,
    rest: {
      issues: {
        listComments: {},
        createComment: async (params: {
          owner: string;
          repo: string;
          issue_number: number;
          body: string;
        }) => {
          created.push({ issue_number: params.issue_number, body: params.body });
        },
      },
    },
  };

  return { github, created };
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

const context = { repo: { owner: "gruntwork-io", repo: "terragrunt" }, runId: 99 };

describe("buildComment", () => {
  test("includes the version, run URL, and install command", () => {
    const comment = buildComment({
      owner: "gruntwork-io",
      repo: "terragrunt",
      commitSha: SHA,
      runId: 99,
    });

    expect(comment).toContain(`\`test-${SHA}\``);
    expect(comment).toContain(
      "https://github.com/gruntwork-io/terragrunt/actions/runs/99",
    );
    expect(comment).toContain(`bash -s -- --test --commit ${SHA}`);
  });
});

describe("notifyTestBuildPR", () => {
  test("comments on the pull request", async () => {
    const { github, created } = fakeGithub([{ body: "some comment" }]);
    const { core, failures } = fakeCore();

    await notifyTestBuildPR({ github, context, core, prNumber: 42, commitSha: SHA });

    expect(failures).toEqual([]);
    expect(created).toHaveLength(1);
    expect(created[0].issue_number).toBe(42);
    expect(created[0].body).toContain(`\`test-${SHA}\``);
  });

  test("skips when the commit already has a comment", async () => {
    const { github, created } = fakeGithub([
      { body: `Available as \`test-${SHA}\`` },
    ]);
    const { core, failures } = fakeCore();

    await notifyTestBuildPR({ github, context, core, prNumber: 42, commitSha: SHA });

    expect(failures).toEqual([]);
    expect(created).toEqual([]);
  });

  test("comments again for a new commit", async () => {
    const { github, created } = fakeGithub([
      { body: "Available as `test-differentsha`" },
    ]);
    const { core } = fakeCore();

    await notifyTestBuildPR({ github, context, core, prNumber: 42, commitSha: SHA });

    expect(created).toHaveLength(1);
  });

  test("fails the step when the comment cannot be posted", async () => {
    const { github } = fakeGithub([]);
    github.rest.issues.createComment = async () => {
      throw new Error("forbidden");
    };
    const { core, failures } = fakeCore();

    await notifyTestBuildPR({ github, context, core, prNumber: 42, commitSha: SHA });

    expect(failures).toEqual(["Failed to comment on PR #42: forbidden"]);
  });
});
