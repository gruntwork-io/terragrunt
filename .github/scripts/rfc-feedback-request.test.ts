import { describe, test, expect } from "bun:test";
import requestRFCFeedback, {
  buildMessage,
  listOpenRFCs,
  postToDiscord,
  selectRFCs,
  MAX_TITLE_LENGTH,
  type Fetch,
  type Issue,
  type RFC,
} from "./rfc-feedback-request";

const WEBHOOK_URL = "https://discord.invalid/api/webhooks/1/fake-token";

function rfc(number: number, thumbsUp: number, updatedAt = "2026-01-01T00:00:00Z"): RFC {
  return {
    number,
    title: `RFC ${number}`,
    url: `https://github.com/gruntwork-io/terragrunt/issues/${number}`,
    thumbsUp,
    updatedAt,
  };
}

function issue(number: number, thumbsUp: number): Issue {
  return {
    number,
    title: `RFC ${number}`,
    html_url: `https://github.com/gruntwork-io/terragrunt/issues/${number}`,
    updated_at: "2026-01-01T00:00:00Z",
    reactions: { "+1": thumbsUp },
  };
}

function githubWith(issues: Issue[]) {
  const calls: unknown[] = [];
  return {
    calls,
    paginate: async (_method: unknown, opts: unknown) => {
      calls.push(opts);
      return issues;
    },
    rest: { issues: { listForRepo: "listForRepo" } },
  };
}

function recordingCore() {
  const info: string[] = [];
  const failed: string[] = [];
  return {
    infoLog: info,
    failed,
    info: (msg: string) => {
      info.push(msg);
    },
    setFailed: (msg: string) => {
      failed.push(msg);
    },
  };
}

function recordingFetch(response = { ok: true, status: 204 }) {
  const requests: Array<{ url: string; body: any }> = [];
  const fetch: Fetch = async (url, init) => {
    requests.push({ url, body: JSON.parse(init.body) });
    return response;
  };
  return { requests, fetch };
}

const CONTEXT = { repo: { owner: "gruntwork-io", repo: "terragrunt" } };

describe("selectRFCs", () => {
  test("picks the three most and three least supported", () => {
    const rfcs = [rfc(1, 5), rfc(2, 0), rfc(3, 24), rfc(4, 1), rfc(5, 10), rfc(6, 2), rfc(7, 7)];

    const { most, least } = selectRFCs(rfcs);

    expect(most.map((r) => r.number)).toEqual([3, 5, 7]);
    expect(least.map((r) => r.number)).toEqual([2, 4, 6]);
  });

  test("breaks ties among the least supported by oldest update", () => {
    const rfcs = [
      rfc(1, 9),
      rfc(2, 8),
      rfc(3, 7),
      rfc(4, 0, "2026-03-01T00:00:00Z"),
      rfc(5, 0, "2026-01-01T00:00:00Z"),
      rfc(6, 0, "2026-02-01T00:00:00Z"),
      rfc(7, 0, "2026-04-01T00:00:00Z"),
    ];

    const { least } = selectRFCs(rfcs);

    expect(least.map((r) => r.number)).toEqual([5, 6, 4]);
  });

  test("lists an RFC once when fewer than six are open", () => {
    const rfcs = [rfc(1, 4), rfc(2, 3), rfc(3, 2), rfc(4, 1)];

    const { most, least } = selectRFCs(rfcs);

    expect(most.map((r) => r.number)).toEqual([1, 2, 3]);
    expect(least.map((r) => r.number)).toEqual([4]);
  });

  test("leaves the input order untouched", () => {
    const rfcs = [rfc(1, 0), rfc(2, 5)];

    selectRFCs(rfcs);

    expect(rfcs.map((r) => r.number)).toEqual([1, 2]);
  });
});

describe("buildMessage", () => {
  test("lists both groups with links and counts", () => {
    const message = buildMessage({ most: [rfc(3, 24)], least: [rfc(2, 0)] });

    expect(message).toBe(
      [
        "**RFC feedback wanted**",
        "",
        "These open RFCs have the most 👍. What would make them work for you?",
        "- [#3](https://github.com/gruntwork-io/terragrunt/issues/3) RFC 3 (👍 24)",
        "",
        "These have the fewest. Would you use them?",
        "- [#2](https://github.com/gruntwork-io/terragrunt/issues/2) RFC 2 (👍 0)",
        "",
        "Add a 👍 or a comment on the issue to weigh in.",
      ].join("\n"),
    );
  });

  test("omits the least supported section when it is empty", () => {
    const message = buildMessage({ most: [rfc(3, 24)], least: [] });

    expect(message).not.toContain("fewest");
  });

  test("truncates long titles", () => {
    const long = { ...rfc(1, 0), title: "x".repeat(MAX_TITLE_LENGTH + 50) };

    const message = buildMessage({ most: [long], least: [] });

    expect(message).toContain(` ${"x".repeat(MAX_TITLE_LENGTH - 1)}… (👍 0)`);
  });

  test("fits Discord's limit with six maximum-length titles", () => {
    const long = (n: number) => ({ ...rfc(n, 1000), title: "x".repeat(500) });

    const message = buildMessage({
      most: [long(10001), long(10002), long(10003)],
      least: [long(10004), long(10005), long(10006)],
    });

    expect(message.length).toBeLessThanOrEqual(2000);
  });
});

describe("listOpenRFCs", () => {
  test("queries open issues with the rfc label", async () => {
    const github = githubWith([]);

    await listOpenRFCs({ github, owner: "gruntwork-io", repo: "terragrunt" });

    expect(github.calls).toEqual([
      { owner: "gruntwork-io", repo: "terragrunt", labels: "rfc", state: "open", per_page: 100 },
    ]);
  });

  test("skips pull requests", async () => {
    const github = githubWith([issue(1, 2), { ...issue(2, 9), pull_request: {} }]);

    const rfcs = await listOpenRFCs({ github, owner: "gruntwork-io", repo: "terragrunt" });

    expect(rfcs.map((r) => r.number)).toEqual([1]);
  });

  test("counts an issue without reactions as zero", async () => {
    const github = githubWith([{ ...issue(1, 0), reactions: undefined }]);

    const rfcs = await listOpenRFCs({ github, owner: "gruntwork-io", repo: "terragrunt" });

    expect(rfcs[0].thumbsUp).toBe(0);
  });
});

describe("postToDiscord", () => {
  test("posts the message with embeds and mentions disabled", async () => {
    const { requests, fetch } = recordingFetch();

    await postToDiscord({ fetch, webhookUrl: WEBHOOK_URL, message: "hello @everyone" });

    expect(requests).toEqual([
      {
        url: WEBHOOK_URL,
        body: { content: "hello @everyone", flags: 4, allowed_mentions: { parse: [] } },
      },
    ]);
  });

  test("reports the status of a rejected post", async () => {
    const { fetch } = recordingFetch({ ok: false, status: 429 });

    await expect(postToDiscord({ fetch, webhookUrl: WEBHOOK_URL, message: "hello" })).rejects.toThrow(
      "Discord webhook responded with status 429",
    );
  });

  test("keeps the webhook URL out of a network error", async () => {
    const fetch: Fetch = async (url) => {
      throw new TypeError(`Failed to parse URL from ${url}`);
    };

    const error = await postToDiscord({ fetch, webhookUrl: WEBHOOK_URL, message: "hello" }).catch(
      (err: Error) => err,
    );

    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).not.toContain("fake-token");
  });
});

describe("requestRFCFeedback", () => {
  test("posts the ranked RFCs", async () => {
    const core = recordingCore();
    const { requests, fetch } = recordingFetch();

    await requestRFCFeedback({
      github: githubWith([issue(1, 0), issue(2, 5), issue(3, 1), issue(4, 9)]),
      context: CONTEXT,
      core,
      fetch,
      webhookUrl: WEBHOOK_URL,
      dryRun: false,
    });

    expect(core.failed).toEqual([]);
    expect(requests).toHaveLength(1);
    expect(requests[0].body.content).toContain("RFC 4 (👍 9)");
    expect(requests[0].body.content).toContain("RFC 1 (👍 0)");
  });

  test("posts nothing on a dry run", async () => {
    const core = recordingCore();
    const { requests, fetch } = recordingFetch();

    await requestRFCFeedback({
      github: githubWith([issue(1, 0)]),
      context: CONTEXT,
      core,
      fetch,
      webhookUrl: "",
      dryRun: true,
    });

    expect(core.failed).toEqual([]);
    expect(requests).toEqual([]);
    expect(core.infoLog.join("\n")).toContain("RFC 1 (👍 0)");
  });

  test("posts nothing when no RFCs are open", async () => {
    const core = recordingCore();
    const { requests, fetch } = recordingFetch();

    await requestRFCFeedback({
      github: githubWith([]),
      context: CONTEXT,
      core,
      fetch,
      webhookUrl: WEBHOOK_URL,
      dryRun: false,
    });

    expect(core.failed).toEqual([]);
    expect(requests).toEqual([]);
  });

  test("fails when the webhook URL is missing", async () => {
    const core = recordingCore();
    const { requests, fetch } = recordingFetch();

    await requestRFCFeedback({
      github: githubWith([issue(1, 0)]),
      context: CONTEXT,
      core,
      fetch,
      webhookUrl: "",
      dryRun: false,
    });

    expect(core.failed).toEqual(["The Discord webhook URL is not set."]);
    expect(requests).toEqual([]);
  });

  test("fails when Discord rejects the post", async () => {
    const core = recordingCore();
    const { fetch } = recordingFetch({ ok: false, status: 500 });

    await requestRFCFeedback({
      github: githubWith([issue(1, 0)]),
      context: CONTEXT,
      core,
      fetch,
      webhookUrl: WEBHOOK_URL,
      dryRun: false,
    });

    expect(core.failed).toEqual([
      "Failed to request RFC feedback: Discord webhook responded with status 500",
    ]);
  });
});
