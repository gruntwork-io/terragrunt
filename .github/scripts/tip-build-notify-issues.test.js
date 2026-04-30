const { describe, test, expect } = require("bun:test");
const {
  parseIssueReferences,
  buildComment,
  hasExistingComment,
  findMergedPRs,
} = require("./tip-build-notify-issues");

const DEFAULT_OWNER = "gruntwork-io";
const DEFAULT_REPO = "terragrunt";

describe("parseIssueReferences", () => {
  test("single shorthand ref with each keyword variant", () => {
    for (const keyword of [
      "close",
      "closes",
      "closed",
      "fix",
      "fixes",
      "fixed",
      "resolve",
      "resolves",
      "resolved",
    ]) {
      const body = `${keyword} #42`;
      const refs = parseIssueReferences(body, DEFAULT_OWNER, DEFAULT_REPO);
      expect(refs).toEqual([
        { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 42 },
      ]);
    }
  });

  test("case insensitive matching", () => {
    const refs = parseIssueReferences(
      "FIXES #1\nCloses #2\nrEsOlVeD #3",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 1 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 2 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 3 },
    ]);
  });

  test("multiple refs after a single keyword", () => {
    const refs = parseIssueReferences(
      "fixes #10, #20, #30",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 10 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 20 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 30 },
    ]);
  });

  test("multiple refs separated by 'and'", () => {
    const refs = parseIssueReferences(
      "closes #1 and #2",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 1 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 2 },
    ]);
  });

  test("multiple refs separated by '&'", () => {
    const refs = parseIssueReferences(
      "fixes #5 & #6",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 5 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 6 },
    ]);
  });

  test("full URL form", () => {
    const refs = parseIssueReferences(
      "closes https://github.com/acme/widgets/issues/99",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([{ owner: "acme", repo: "widgets", number: 99 }]);
  });

  test("mixed URL and shorthand refs", () => {
    const refs = parseIssueReferences(
      "fixes https://github.com/acme/widgets/issues/1, #2",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: "acme", repo: "widgets", number: 1 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 2 },
    ]);
  });

  test("multiple keywords on separate lines", () => {
    const body = [
      "fixes #100",
      "also resolves #200",
      "closes #300",
    ].join("\n");
    const refs = parseIssueReferences(body, DEFAULT_OWNER, DEFAULT_REPO);
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 100 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 200 },
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 300 },
    ]);
  });

  test("deduplicates identical refs", () => {
    const refs = parseIssueReferences(
      "fixes #1, #1\ncloses #1",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([
      { owner: DEFAULT_OWNER, repo: DEFAULT_REPO, number: 1 },
    ]);
  });

  test("word boundary prevents matching inside words like 'unresolved'", () => {
    const refs = parseIssueReferences(
      "this is unresolved #123",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([]);
  });

  test("word boundary prevents matching 'prefix_fix'", () => {
    const refs = parseIssueReferences(
      "prefix_fix #5",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([]);
  });

  test("word boundary prevents matching 'quickfix'", () => {
    const refs = parseIssueReferences(
      "quickfix #7",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([]);
  });

  test("no keywords returns empty array", () => {
    const refs = parseIssueReferences(
      "This PR adds feature #42",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([]);
  });

  test("keyword with no ref returns empty array", () => {
    const refs = parseIssueReferences(
      "This fixes the flaky test",
      DEFAULT_OWNER,
      DEFAULT_REPO,
    );
    expect(refs).toEqual([]);
  });

  test("empty body returns empty array", () => {
    expect(parseIssueReferences("", DEFAULT_OWNER, DEFAULT_REPO)).toEqual([]);
  });
});

describe("buildComment", () => {
  test("includes commit sha, PR links, and run URL", () => {
    const comment = buildComment({
      owner: "gruntwork-io",
      repo: "terragrunt",
      commitSha: "abc123",
      prNumbers: [42],
      runId: 99,
    });

    expect(comment).toContain("`tip-abc123`");
    expect(comment).toContain("#42");
    expect(comment).toContain(
      "https://github.com/gruntwork-io/terragrunt/actions/runs/99",
    );
    expect(comment).toContain("install the tip build");
  });

  test("lists multiple PR numbers", () => {
    const comment = buildComment({
      owner: "o",
      repo: "r",
      commitSha: "sha",
      prNumbers: [1, 2, 3],
      runId: 1,
    });

    expect(comment).toContain("#1, #2, #3");
  });
});

describe("hasExistingComment", () => {
  test("returns true when marker comment exists", async () => {
    const github = {
      paginate: async (_method, _opts) => [
        { body: "some comment" },
        { body: "Included in `tip-abc123` build" },
      ],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      commitSha: "abc123",
    });
    expect(result).toBe(true);
  });

  test("returns false when no marker comment exists", async () => {
    const github = {
      paginate: async (_method, _opts) => [
        { body: "some comment" },
        { body: "tip-differentsha" },
      ],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      commitSha: "abc123",
    });
    expect(result).toBe(false);
  });

  test("returns false for empty comments list", async () => {
    const github = {
      paginate: async (_method, _opts) => [],
      rest: { issues: { listComments: {} } },
    };

    const result = await hasExistingComment({
      github,
      owner: "o",
      repo: "r",
      issueNumber: 1,
      commitSha: "abc123",
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
      commitSha: "sha",
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

describe("findMergedPRs", () => {
  test("returns only merged PRs", async () => {
    const github = {
      rest: {
        repos: {
          listPullRequestsAssociatedWithCommit: async () => ({
            data: [
              { number: 1, merged_at: "2024-01-01T00:00:00Z" },
              { number: 2, merged_at: null },
              { number: 3, merged_at: "2024-01-02T00:00:00Z" },
            ],
          }),
        },
      },
    };

    const result = await findMergedPRs({
      github,
      owner: "o",
      repo: "r",
      commitSha: "sha",
    });
    expect(result).toEqual([
      { number: 1, merged_at: "2024-01-01T00:00:00Z" },
      { number: 3, merged_at: "2024-01-02T00:00:00Z" },
    ]);
  });

  test("returns empty array when no PRs are merged", async () => {
    const github = {
      rest: {
        repos: {
          listPullRequestsAssociatedWithCommit: async () => ({
            data: [{ number: 1, merged_at: null }],
          }),
        },
      },
    };

    const result = await findMergedPRs({
      github,
      owner: "o",
      repo: "r",
      commitSha: "sha",
    });
    expect(result).toEqual([]);
  });
});
