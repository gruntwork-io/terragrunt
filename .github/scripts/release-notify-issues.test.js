const { describe, test, expect } = require("bun:test");
const {
  mapIssuesToPRs,
  buildComment,
  hasExistingComment,
  collectMergedPRs,
  listCommitsInRange,
  MARKER_PREFIX,
} = require("./release-notify-issues");

const DEFAULT_OWNER = "gruntwork-io";
const DEFAULT_REPO = "terragrunt";

describe("mapIssuesToPRs", () => {
  test("maps each closed issue to the PRs that closed it", () => {
    const mergedPRs = [
      { number: 100, body: "fixes #1" },
      { number: 101, body: "closes #2 and #3" },
    ];
    const result = mapIssuesToPRs({
      mergedPRs,
      owner: DEFAULT_OWNER,
      repo: DEFAULT_REPO,
    });
    expect([...result.entries()]).toEqual([
      [1, [100]],
      [2, [101]],
      [3, [101]],
    ]);
  });

  test("aggregates multiple PRs that close the same issue", () => {
    const mergedPRs = [
      { number: 100, body: "fixes #1" },
      { number: 101, body: "also fixes #1" },
    ];
    const result = mapIssuesToPRs({
      mergedPRs,
      owner: DEFAULT_OWNER,
      repo: DEFAULT_REPO,
    });
    expect(result.get(1)).toEqual([100, 101]);
  });

  test("skips PRs without a body", () => {
    const mergedPRs = [
      { number: 100, body: null },
      { number: 101, body: "" },
      { number: 102, body: "closes #5" },
    ];
    const result = mapIssuesToPRs({
      mergedPRs,
      owner: DEFAULT_OWNER,
      repo: DEFAULT_REPO,
    });
    expect([...result.entries()]).toEqual([[5, [102]]]);
  });

  test("skips cross-repo references and warns", () => {
    const warnings = [];
    const core = { warning: (msg) => warnings.push(msg) };
    const mergedPRs = [
      {
        number: 100,
        body: "closes https://github.com/acme/widgets/issues/9 and #2",
      },
    ];
    const result = mapIssuesToPRs({
      mergedPRs,
      owner: DEFAULT_OWNER,
      repo: DEFAULT_REPO,
      core,
    });
    expect([...result.entries()]).toEqual([[2, [100]]]);
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("acme/widgets#9");
    expect(warnings[0]).toContain("#100");
  });

  test("returns empty map when no PRs reference issues", () => {
    const mergedPRs = [
      { number: 100, body: "improves logging" },
      { number: 101, body: "refactors helper" },
    ];
    const result = mapIssuesToPRs({
      mergedPRs,
      owner: DEFAULT_OWNER,
      repo: DEFAULT_REPO,
    });
    expect(result.size).toBe(0);
  });
});

describe("buildComment", () => {
  test("includes tag, release URL, PR links, and HTML marker", () => {
    const comment = buildComment({
      owner: "gruntwork-io",
      repo: "terragrunt",
      tagName: "v0.96.0",
      prNumbers: [42],
    });

    expect(comment).toContain("`v0.96.0`");
    expect(comment).toContain(
      "https://github.com/gruntwork-io/terragrunt/releases/tag/v0.96.0",
    );
    expect(comment).toContain("#42");
    expect(comment).toContain("install Terragrunt");
    expect(comment).toContain(`<!-- ${MARKER_PREFIX} v0.96.0 -->`);
  });

  test("lists multiple PR numbers", () => {
    const comment = buildComment({
      owner: "o",
      repo: "r",
      tagName: "v1.0.0",
      prNumbers: [1, 2, 3],
    });
    expect(comment).toContain("#1, #2, #3");
  });
});

describe("hasExistingComment", () => {
  test("returns true when marker comment exists", async () => {
    const github = {
      paginate: async () => [
        { body: "thanks for the fix!" },
        { body: `Released. <!-- ${MARKER_PREFIX} v0.96.0 -->` },
      ],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      tagName: "v0.96.0",
    });
    expect(result).toBe(true);
  });

  test("returns false when only a different tag's marker exists", async () => {
    const github = {
      paginate: async () => [
        { body: `<!-- ${MARKER_PREFIX} v0.95.0 -->` },
      ],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      tagName: "v0.96.0",
    });
    expect(result).toBe(false);
  });

  test("returns false for empty comments list", async () => {
    const github = {
      paginate: async () => [],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      tagName: "v0.96.0",
    });
    expect(result).toBe(false);
  });

  test("passes correct parameters to paginate", async () => {
    let capturedMethod, capturedOpts;
    const listCommentsFn = () => {};
    const github = {
      paginate: async (method, opts) => {
        capturedMethod = method;
        capturedOpts = opts;
        return [];
      },
      rest: { issues: { listComments: listCommentsFn } },
    };

    await hasExistingComment({
      github,
      owner: "gruntwork-io",
      repo: "terragrunt",
      issueNumber: 42,
      tagName: "v0.96.0",
    });

    expect(capturedMethod).toBe(listCommentsFn);
    expect(capturedOpts).toEqual({
      owner: "gruntwork-io",
      repo: "terragrunt",
      issue_number: 42,
      per_page: 100,
    });
  });
});

describe("collectMergedPRs", () => {
  test("deduplicates PRs across commits and drops unmerged ones", async () => {
    const prsByCommit = {
      sha1: [
        { number: 1, merged_at: "2024-01-01T00:00:00Z" },
        { number: 2, merged_at: null },
      ],
      sha2: [
        { number: 1, merged_at: "2024-01-01T00:00:00Z" },
        { number: 3, merged_at: "2024-01-02T00:00:00Z" },
      ],
    };
    const github = {
      rest: {
        repos: {
          listPullRequestsAssociatedWithCommit: async ({ commit_sha }) => ({
            data: prsByCommit[commit_sha] || [],
          }),
        },
      },
    };

    const result = await collectMergedPRs({
      github,
      owner: "o",
      repo: "r",
      commits: [{ sha: "sha1" }, { sha: "sha2" }],
    });
    expect(result.map((pr) => pr.number)).toEqual([1, 3]);
  });

  test("returns empty array when no commits have merged PRs", async () => {
    const github = {
      rest: {
        repos: {
          listPullRequestsAssociatedWithCommit: async () => ({
            data: [{ number: 1, merged_at: null }],
          }),
        },
      },
    };

    const result = await collectMergedPRs({
      github,
      owner: "o",
      repo: "r",
      commits: [{ sha: "sha1" }],
    });
    expect(result).toEqual([]);
  });
});

describe("listCommitsInRange", () => {
  test("uses paginate with the basehead form and 100 per_page", async () => {
    let capturedMethod, capturedOpts;
    const compareFn = () => {};
    const github = {
      paginate: async (method, opts) => {
        capturedMethod = method;
        capturedOpts = opts;
        return [{ sha: "a" }, { sha: "b" }];
      },
      rest: { repos: { compareCommitsWithBasehead: compareFn } },
    };

    const result = await listCommitsInRange({
      github,
      owner: "gruntwork-io",
      repo: "terragrunt",
      base: "v0.95.0",
      head: "v0.96.0",
    });

    expect(result).toEqual([{ sha: "a" }, { sha: "b" }]);
    expect(capturedMethod).toBe(compareFn);
    expect(capturedOpts).toEqual({
      owner: "gruntwork-io",
      repo: "terragrunt",
      basehead: "v0.95.0...v0.96.0",
      per_page: 100,
    });
  });
});
