import { describe, test, expect } from "bun:test";
import retargetOpenPRs, {
  addedLineNumbers,
  nextPatchVersion,
  planRetarget,
  retargetFrontmatter,
  retargetGates,
  type PullRequest,
  type PullRequestFile,
  type RetargetClient,
  type RetargetPullRequest,
} from "./docs-retarget-open-prs";

const OWNER = "gruntwork-io";
const REPO = "terragrunt";
const NOW = new Date("2026-11-11T00:00:00Z");

const ENTRY = [
  "---",
  'version: "v1.2.2"',
  'category: "bug-fixes"',
  "---",
  "",
  "#### Auto-init detects module changes",
  "",
].join("\n");

const PAGE = [
  "- Always shown.",
  "",
  '<Before version="v1.2.1">',
  "",
  "- From main.",
  "",
  "</Before>",
  "",
  '<Since version="v1.2.2">',
  "",
  "- From the PR.",
  "",
  "</Since>",
  "",
].join("\n");

const PAGE_PATCH = [
  "@@ -1,7 +1,13 @@",
  " - Always shown.",
  " ",
  ' <Before version="v1.2.1">',
  " ",
  " - From main.",
  " ",
  " </Before>",
  "+",
  '+<Since version="v1.2.2">',
  "+",
  "+- From the PR.",
  "+",
  "+</Since>",
].join("\n");

describe("nextPatchVersion", () => {
  test("increments the patch component", () => {
    expect(nextPatchVersion("v1.2.2")).toBe("v1.2.3");
    expect(nextPatchVersion("v1.2.9")).toBe("v1.2.10");
  });

  test("keeps a bare version bare", () => {
    expect(nextPatchVersion("1.0.7")).toBe("1.0.8");
  });

  test("rejects a prerelease tag", () => {
    expect(() => nextPatchVersion("v1.3.0-rc1")).toThrow("v1.3.0-rc1");
  });
});

describe("addedLineNumbers", () => {
  test("numbers added lines by their position in the new file", () => {
    const patch = [
      "@@ -1,3 +1,4 @@",
      " one",
      "-two",
      "+2",
      "+2.5",
      " three",
      "@@ -10,2 +11,3 @@ heading",
      " ten",
      "+eleven",
      "\\ No newline at end of file",
    ].join("\n");
    expect([...addedLineNumbers(patch)]).toEqual([2, 3, 12]);
  });
});

describe("retargetGates", () => {
  test("bumps released gates on added lines only", () => {
    const added = addedLineNumbers(PAGE_PATCH);
    const result = retargetGates({
      source: PAGE,
      isAdded: (line) => added.has(line),
      latestTag: "v1.2.2",
      nextTag: "v1.2.3",
    });
    expect(result).toBe(PAGE.replace('"v1.2.2"', '"v1.2.3"'));
  });

  test("leaves a gate for an unreleased version alone", () => {
    const source = '<Since version="v1.3.0">new</Since>';
    const result = retargetGates({
      source,
      isAdded: () => true,
      latestTag: "v1.2.2",
      nextTag: "v1.2.3",
    });
    expect(result).toBe(source);
  });

  test("bumps both gates of an inline pair and keeps a bare version bare", () => {
    const result = retargetGates({
      source:
        "Path is <Before version='1.2.2'>`old`</Before><Since version=\"1.2.0\">`new`</Since>.",
      isAdded: () => true,
      latestTag: "v1.2.2",
      nextTag: "v1.2.3",
    });
    expect(result).toBe(
      "Path is <Before version='1.2.3'>`old`</Before><Since version=\"1.2.3\">`new`</Since>.",
    );
  });
});

describe("retargetFrontmatter", () => {
  test("rewrites a released version", () => {
    const result = retargetFrontmatter({
      source: ENTRY,
      latestTag: "v1.2.2",
      nextTag: "v1.2.3",
    });
    expect(result).toBe(ENTRY.replace("v1.2.2", "v1.2.3"));
  });

  test("ignores a version line in the body", () => {
    const source = `${ENTRY}\nversion: "v1.2.2"\n`;
    const result = retargetFrontmatter({
      source,
      latestTag: "v1.2.2",
      nextTag: "v1.2.3",
    });
    expect(result).toBe(`${ENTRY.replace("v1.2.2", "v1.2.3")}\nversion: "v1.2.2"\n`);
  });
});

function file(overrides: Partial<PullRequestFile> & { filename: string }): PullRequestFile {
  return { status: "added", changes: 1, ...overrides };
}

async function plan(files: PullRequestFile[], contents: Record<string, string>) {
  const warnings: string[] = [];
  const changes = await planRetarget({
    files,
    latestTag: "v1.2.2",
    nextTag: "v1.2.3",
    readFile: async (path) => {
      expect(contents).toHaveProperty([path]);
      return contents[path];
    },
    exists: async (path) => path in contents,
    warn: (message) => warnings.push(message),
  });
  return { changes, warnings };
}

describe("planRetarget", () => {
  test("moves an added changelog entry to the next patch release", async () => {
    const path = "docs/src/data/changelog/v1.2.2/auto-init.mdx";
    const { changes } = await plan([file({ filename: path })], { [path]: ENTRY });
    expect(changes).toEqual({
      additions: [
        {
          path: "docs/src/data/changelog/v1.2.3/auto-init.mdx",
          contents: ENTRY.replace("v1.2.2", "v1.2.3"),
        },
      ],
      deletions: [{ path }],
    });
  });

  test("leaves an edit to a released changelog entry in place", async () => {
    const path = "docs/src/data/changelog/v1.2.2/auto-init.mdx";
    const { changes } = await plan(
      [file({ filename: path, status: "modified", patch: "@@ -6 +6 @@\n-old\n+new" })],
      { [path]: ENTRY },
    );
    expect(changes).toEqual({ additions: [], deletions: [] });
  });

  test("leaves a changelog entry for an unreleased version in place", async () => {
    const path = "docs/src/data/changelog/v1.3.0/feature.mdx";
    const source = ENTRY.replace("v1.2.2", "v1.3.0");
    const { changes } = await plan([file({ filename: path })], { [path]: source });
    expect(changes).toEqual({ additions: [], deletions: [] });
  });

  test("bumps gates a modified page adds", async () => {
    const path = "docs/src/content/docs/03-features/auto-init.mdx";
    const { changes } = await plan(
      [file({ filename: path, status: "modified", patch: PAGE_PATCH })],
      { [path]: PAGE },
    );
    expect(changes).toEqual({
      additions: [{ path, contents: PAGE.replace('"v1.2.2"', '"v1.2.3"') }],
      deletions: [],
    });
  });

  test("ignores removed files and files outside docs/src", async () => {
    const { changes } = await plan(
      [
        file({ filename: "docs/src/data/changelog/v1.2.2/gone.mdx", status: "removed" }),
        file({ filename: "internal/tf/modules.go" }),
      ],
      {},
    );
    expect(changes).toEqual({ additions: [], deletions: [] });
  });

  test("does not overwrite an entry the PR already has at the destination", async () => {
    const from = "docs/src/data/changelog/v1.2.2/auto-init.mdx";
    const to = "docs/src/data/changelog/v1.2.3/auto-init.mdx";
    const { changes, warnings } = await plan(
      [file({ filename: from }), file({ filename: to })],
      { [from]: ENTRY, [to]: ENTRY.replace("v1.2.2", "v1.2.3") },
    );
    expect(changes).toEqual({ additions: [], deletions: [] });
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain(to);
  });

  test("does not overwrite an entry the PR head has at the destination", async () => {
    const from = "docs/src/data/changelog/v1.2.2/auto-init.mdx";
    const to = "docs/src/data/changelog/v1.2.3/auto-init.mdx";
    const { changes, warnings } = await plan([file({ filename: from })], {
      [from]: ENTRY,
      [to]: "from main",
    });
    expect(changes).toEqual({ additions: [], deletions: [] });
    expect(warnings).toEqual([expect.stringContaining(to)]);
  });

  test("moves only the first of two entries bound for the same destination", async () => {
    const first = "docs/src/data/changelog/v1.2.1/auto-init.mdx";
    const second = "docs/src/data/changelog/v1.2.2/auto-init.mdx";
    const { changes, warnings } = await plan(
      [file({ filename: first }), file({ filename: second })],
      { [first]: ENTRY.replace("v1.2.2", "v1.2.1"), [second]: ENTRY },
    );
    expect(changes).toEqual({
      additions: [
        {
          path: "docs/src/data/changelog/v1.2.3/auto-init.mdx",
          contents: ENTRY.replace("v1.2.2", "v1.2.3"),
        },
      ],
      deletions: [{ path: first }],
    });
    expect(warnings).toEqual([expect.stringContaining(second)]);
  });

  test("warns when a changed page has no diff", async () => {
    const path = "docs/src/content/docs/big.mdx";
    const { changes, warnings } = await plan(
      [file({ filename: path, status: "modified", changes: 4000 })],
      {},
    );
    expect(changes).toEqual({ additions: [], deletions: [] });
    expect(warnings).toEqual([expect.stringContaining(path)]);
  });
});

type Call = { name: string; params: Record<string, unknown> };

function pr(overrides: Partial<PullRequest> & { number: number }): PullRequest {
  return {
    updated_at: "2026-11-10T00:00:00Z",
    head: {
      ref: `feature-${overrides.number}`,
      sha: `sha-${overrides.number}`,
      repo: { full_name: `${OWNER}/${REPO}` },
    },
    ...overrides,
  };
}

const RETARGET_HEAD = `${OWNER}:docs/retarget-pr-7291`;
const GENERATED_TIP = { sha: "tip-7291", message: "docs: Re-targeting #7291 to `v1.2.2`" };

function fakeGithub({
  prs,
  files,
  contents,
  branchTips = {},
  comparisons = {},
  retargetPRs = {},
}: {
  prs: PullRequest[];
  files: Record<number, PullRequestFile[]>;
  contents: Record<string, string>;
  branchTips?: Record<string, { sha: string; message: string }>;
  comparisons?: Record<string, string>;
  retargetPRs?: Record<string, RetargetPullRequest[]>;
}) {
  const calls: Call[] = [];
  const record = (name: string) => async (params: Record<string, unknown>) => {
    calls.push({ name, params });
    return { data: {} };
  };
  const list = Symbol("pulls.list");
  const listFiles = Symbol("pulls.listFiles");

  const github = {
    paginate: async (method: unknown, opts: Record<string, unknown>) => {
      if (method === listFiles) {
        return files[opts.pull_number as number] ?? [];
      }
      if (typeof opts.head === "string") {
        expect(opts.state).toBe("all");
        return retargetPRs[opts.head] ?? [];
      }
      return prs;
    },
    graphql: async (_query: string, variables: Record<string, unknown>) => {
      calls.push({ name: "graphql", params: variables });
      return {};
    },
    rest: {
      pulls: {
        list,
        listFiles,
        create: async (params: Record<string, unknown>) => {
          calls.push({ name: "pulls.create", params });
          return { data: { number: 9000 } };
        },
        update: record("pulls.update"),
      },
      repos: {
        getContent: async ({ path, ref }: { path: string; ref: string }) => {
          const content = contents[`${ref}:${path}`];
          if (content === undefined) {
            throw Object.assign(new Error("Not Found"), { status: 404 });
          }
          return {
            data: {
              content: Buffer.from(content).toString("base64"),
              encoding: "base64",
            },
          };
        },
        compareCommits: async ({ base, head }: { base: string; head: string }) => ({
          data: { status: comparisons[`${base}...${head}`] ?? "diverged" },
        }),
      },
      git: {
        createRef: async (params: Record<string, unknown>) => {
          const branch = (params.ref as string).replace(/^refs\/heads\//, "");
          if (branch in branchTips) {
            throw Object.assign(new Error("Reference already exists"), { status: 422 });
          }
          calls.push({ name: "git.createRef", params });
          return {};
        },
        getRef: async ({ ref }: { ref: string }) => ({
          data: { object: { sha: branchTips[ref.replace(/^heads\//, "")].sha } },
        }),
        getCommit: async ({ commit_sha }: { commit_sha: string }) => {
          const tip = Object.values(branchTips).find((t) => t.sha === commit_sha);
          if (tip === undefined) {
            throw new Error(`unexpected commit ${commit_sha}`);
          }
          return { data: { message: tip.message, parents: [{ sha: "parent" }] } };
        },
        updateRef: record("git.updateRef"),
      },
      issues: { addLabels: record("issues.addLabels") },
    },
  } as unknown as RetargetClient;

  return { github, calls };
}

function fakeCore() {
  const log = { info: [] as string[], warning: [] as string[], failed: [] as string[] };
  return {
    log,
    core: {
      info: (msg: string) => log.info.push(msg),
      warning: (msg: string) => log.warning.push(msg),
      setFailed: (msg: string) => log.failed.push(msg),
    },
  };
}

const ENTRY_PATH = "docs/src/data/changelog/v1.2.2/auto-init.mdx";

async function runWith(
  fake: ReturnType<typeof fakeGithub>,
  options: { tagName?: string; maxAgeDays?: number } = {},
) {
  const { core, log } = fakeCore();
  await retargetOpenPRs({
    github: fake.github,
    context: { repo: { owner: OWNER, repo: REPO } },
    core,
    tagName: "v1.2.2",
    now: NOW,
    ...options,
  });
  return log;
}

describe("retargetOpenPRs", () => {
  test("opens a chained PR against the stale PR's branch", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
    });
    const log = await runWith(fake);

    expect(log.failed).toEqual([]);
    expect(fake.calls.map((call) => call.name)).toEqual([
      "git.createRef",
      "graphql",
      "pulls.create",
      "issues.addLabels",
    ]);
    expect(fake.calls[0].params).toMatchObject({
      ref: "refs/heads/docs/retarget-pr-7291",
      sha: "sha-7291",
    });
    expect(fake.calls[1].params).toEqual({
      input: {
        branch: {
          repositoryNameWithOwner: `${OWNER}/${REPO}`,
          branchName: "docs/retarget-pr-7291",
        },
        expectedHeadOid: "sha-7291",
        message: { headline: "docs: Re-targeting #7291 to `v1.2.3`" },
        fileChanges: {
          additions: [
            {
              path: "docs/src/data/changelog/v1.2.3/auto-init.mdx",
              contents: Buffer.from(ENTRY.replace("v1.2.2", "v1.2.3")).toString("base64"),
            },
          ],
          deletions: [{ path: ENTRY_PATH }],
        },
      },
    });
    expect(fake.calls[2].params).toMatchObject({
      head: "docs/retarget-pr-7291",
      base: "feature-7291",
      title: "docs: Re-targeting #7291 to `v1.2.3`",
    });
  });

  test("skips PRs older than the cutoff, from forks, and its own PRs", async () => {
    const stale = pr({ number: 1, updated_at: "2026-10-01T00:00:00Z" });
    const fork = pr({ number: 2 });
    fork.head.repo = { full_name: "someone/terragrunt" };
    const deletedFork = pr({ number: 3 });
    deletedFork.head.repo = null;
    const own = pr({ number: 4 });
    own.head.ref = "docs/retarget-pr-7291";

    const fake = fakeGithub({
      prs: [stale, fork, deletedFork, own],
      files: {},
      contents: {},
    });
    const log = await runWith(fake);

    expect(fake.calls).toEqual([]);
    expect(log.failed).toEqual([]);
    expect(log.info.filter((msg) => msg.startsWith("Skipping"))).toHaveLength(4);
  });

  test("honors a custom age limit", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 1, updated_at: "2026-10-01T00:00:00Z" })],
      files: { 1: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-1:${ENTRY_PATH}`]: ENTRY },
    });
    await runWith(fake, { maxAgeDays: 60 });
    expect(fake.calls.map((call) => call.name)).toContain("pulls.create");
  });

  test("leaves a PR with nothing to re-target alone", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 5 })],
      files: { 5: [file({ filename: "internal/tf/modules.go" })] },
      contents: {},
    });
    await runWith(fake);
    expect(fake.calls).toEqual([]);
  });

  test("resets an existing branch and refreshes its open PR", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
      branchTips: { "docs/retarget-pr-7291": GENERATED_TIP },
      retargetPRs: {
        [RETARGET_HEAD]: [{ number: 7400, state: "open", merged_at: null }],
      },
    });
    await runWith(fake);

    expect(fake.calls.map((call) => call.name)).toEqual([
      "git.updateRef",
      "graphql",
      "pulls.update",
    ]);
    expect(fake.calls[0].params).toMatchObject({
      ref: "heads/docs/retarget-pr-7291",
      sha: "sha-7291",
      force: true,
    });
    expect(fake.calls[2].params).toMatchObject({
      pull_number: 7400,
      title: "docs: Re-targeting #7291 to `v1.2.3`",
      body: expect.stringContaining("`v1.2.3`"),
    });
  });

  test("skips a PR whose re-target PR was closed without merging", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
      branchTips: { "docs/retarget-pr-7291": GENERATED_TIP },
      retargetPRs: {
        [RETARGET_HEAD]: [{ number: 7400, state: "closed", merged_at: null }],
      },
    });
    const log = await runWith(fake);

    expect(fake.calls).toEqual([]);
    expect(log.failed).toEqual([]);
    expect(log.info).toContain("Skipping #7291: #7400 was closed without merging.");
  });

  test("opens a new PR after the previous re-target PR merged", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
      branchTips: { "docs/retarget-pr-7291": GENERATED_TIP },
      retargetPRs: {
        [RETARGET_HEAD]: [
          { number: 7400, state: "closed", merged_at: "2026-11-01T00:00:00Z" },
        ],
      },
    });
    await runWith(fake);

    expect(fake.calls.map((call) => call.name)).toEqual([
      "git.updateRef",
      "graphql",
      "pulls.create",
      "issues.addLabels",
    ]);
  });

  test("leaves a re-target branch with someone else's commits alone", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
      branchTips: {
        "docs/retarget-pr-7291": { sha: "tip-7291", message: "Fix a typo" },
      },
      retargetPRs: {
        [RETARGET_HEAD]: [{ number: 7400, state: "open", merged_at: null }],
      },
    });
    const log = await runWith(fake);

    expect(fake.calls).toEqual([]);
    expect(log.failed).toEqual([]);
    expect(log.warning).toEqual([expect.stringContaining("docs/retarget-pr-7291")]);
  });

  test("resets a re-target branch the PR head already contains", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 7291 })],
      files: { 7291: [file({ filename: ENTRY_PATH })] },
      contents: { [`sha-7291:${ENTRY_PATH}`]: ENTRY },
      branchTips: {
        "docs/retarget-pr-7291": { sha: "sha-7290", message: "Earlier PR commit" },
      },
      comparisons: { "sha-7290...sha-7291": "ahead" },
    });
    await runWith(fake);

    expect(fake.calls.map((call) => call.name)).toEqual([
      "git.updateRef",
      "graphql",
      "pulls.create",
      "issues.addLabels",
    ]);
  });

  test("keeps going after one PR fails and reports the failure", async () => {
    const fake = fakeGithub({
      prs: [pr({ number: 1 }), pr({ number: 2 })],
      files: {
        1: [file({ filename: ENTRY_PATH })],
        2: [file({ filename: ENTRY_PATH })],
      },
      contents: { [`sha-2:${ENTRY_PATH}`]: ENTRY },
    });
    const log = await runWith(fake);

    expect(log.warning).toEqual([expect.stringContaining("#1")]);
    expect(log.failed).toEqual(["Failed to re-target 1 PR(s)."]);
    expect(fake.calls.map((call) => call.name)).toContain("pulls.create");
  });

  test("fails on a tag with no next patch release", async () => {
    const fake = fakeGithub({ prs: [], files: {}, contents: {} });
    const log = await runWith(fake, { tagName: "v1.3.0-rc1" });
    expect(log.failed).toEqual([expect.stringContaining("v1.3.0-rc1")]);
  });
});
