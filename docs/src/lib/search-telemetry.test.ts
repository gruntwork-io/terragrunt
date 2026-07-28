import { describe, expect, test } from "bun:test";
import {
  SEARCH_EVENTS,
  createSearchTracker,
  hasSettled,
  kapaEventProperties,
  normalizeQuery,
  parseResultCount,
  redact,
  searchingPrefix,
} from "./search-telemetry";

const TRANSLATIONS = JSON.stringify({
  placeholder: "Search",
  zero_results: "No results for [SEARCH_TERM]",
  many_results: "[COUNT] results for [SEARCH_TERM]",
  searching: "Searching for [SEARCH_TERM]...",
});

const LOADING_PREFIX = "Searching for ";

interface CapturedEvent {
  event: string;
  properties: Record<string, unknown>;
}

function makeTracker() {
  const captured: CapturedEvent[] = [];
  const tracker = createSearchTracker((event, properties) => {
    captured.push({ event, properties });
  });
  return { tracker, captured };
}

describe("normalizeQuery", () => {
  test("trims surrounding whitespace", () => {
    expect(normalizeQuery("  terragrunt run  ")).toBe("terragrunt run");
  });

  test("collapses runs of whitespace", () => {
    expect(normalizeQuery("terragrunt\t\n  run")).toBe("terragrunt run");
  });

  test("returns an empty string for whitespace-only input", () => {
    expect(normalizeQuery("   ")).toBe("");
  });
});

describe("parseResultCount", () => {
  test("reads the total from Pagefind's message", () => {
    expect(parseResultCount("42 results for stacks", 5)).toBe(42);
  });

  test("ignores numbers inside the search term", () => {
    expect(parseResultCount("No results for opentofu 1.5", 0)).toBe(0);
  });

  test("falls back to the rendered count when the message has no total", () => {
    expect(parseResultCount("No results for stakcs. Showing results for stacks instead", 5)).toBe(5);
  });

  test("falls back to the rendered count when there is no message", () => {
    expect(parseResultCount(null, 3)).toBe(3);
  });
});

describe("redact", () => {
  test("leaves ordinary searches alone", () => {
    const searches = [
      "terragrunt run --all apply",
      "registry.opentofu.org/hashicorp/aws",
      "arn:aws:iam::123456789012:role/terragrunt",
      "S3_BUCKET_NAME",
      "how do I configure a state backend?",
      "terragrunt.stack.hcl",
    ];

    for (const search of searches) expect(redact(search)).toBe(search);
  });

  test("removes email addresses", () => {
    expect(redact("invite example@acme.com to the org")).toBe("invite [email] to the org");
  });

  test("removes AWS access key ids", () => {
    expect(redact("AKIAIOSFODNN7EXAMPLE denied")).toBe("[redacted] denied");
  });

  test("removes GitHub tokens", () => {
    expect(redact("ghp_" + "a1b2c3d4e5".repeat(3) + " expired")).toBe("[redacted] expired");
  });

  test("removes Slack tokens", () => {
    expect(redact("xoxb-1234567890-abcdefghij")).toBe("[redacted]");
  });

  test("removes JSON web tokens", () => {
    const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U";
    expect(redact(`auth failing with ${jwt}`)).toBe("auth failing with [redacted]");
  });

  test("removes private key blocks", () => {
    expect(redact("-----BEGIN RSA PRIVATE KEY-----")).toBe("[redacted]");
  });

  test("removes the value assigned to a secret-ish name, keeping the name", () => {
    expect(redact("password=hunter2")).toBe("password=[redacted]");
    expect(redact("api_key: abc123")).toBe("api_key: [redacted]");
    expect(redact("TOKEN = xyz")).toBe("TOKEN = [redacted]");
  });

  test("removes long generated-looking strings", () => {
    expect(redact("wJalrXUtnFEMI1K7MDENG2bPxRfiCYEXAMPLEKEY")).toBe("[redacted]");
  });

  test("keeps long strings that are not mixed enough to be credentials", () => {
    const slug = "terragrunt-stack-configuration-and-orchestration";
    expect(redact(slug)).toBe(slug);
  });
});

describe("searchingPrefix", () => {
  test("takes the fixed part of Pagefind's searching message", () => {
    expect(searchingPrefix(TRANSLATIONS)).toBe(LOADING_PREFIX);
  });

  test("works for a translation that leads with the search term", () => {
    expect(searchingPrefix(JSON.stringify({ searching: "[SEARCH_TERM] wird gesucht..." }))).toBe("");
  });

  test("falls back to Pagefind's own wording when the site has not overridden it", () => {
    expect(searchingPrefix(null)).toBe(LOADING_PREFIX);
    expect(searchingPrefix("not json")).toBe(LOADING_PREFIX);
    expect(searchingPrefix(JSON.stringify({ placeholder: "Search" }))).toBe(LOADING_PREFIX);
  });
});

describe("hasSettled", () => {
  test("treats a result count for the current query as settled", () => {
    expect(hasSettled("28 results for lock file", "lock file", LOADING_PREFIX)).toBe(true);
  });

  test("treats no results for the current query as settled", () => {
    expect(hasSettled("No results for lock file", "lock file", LOADING_PREFIX)).toBe(true);
  });

  test("does not treat Pagefind's searching message as settled", () => {
    expect(hasSettled("Searching for lock file...", "lock file", LOADING_PREFIX)).toBe(false);
  });

  test("does not treat a missing message as settled", () => {
    expect(hasSettled(null, "lock file", LOADING_PREFIX)).toBe(false);
  });

  test("does not treat the previous query's message as settled", () => {
    expect(hasSettled("28 results for lock file", "provider cache", LOADING_PREFIX)).toBe(false);
  });

  test("ignores case when matching the query", () => {
    expect(hasSettled("28 results for Lock File", "lock file", LOADING_PREFIX)).toBe(true);
  });

  test("keeps matching when no loading prefix could be derived", () => {
    expect(hasSettled("28 results for lock file", "lock file", "")).toBe(true);
  });

  test("treats a leading total as final even when it starts with the loading prefix", () => {
    expect(hasSettled("28 results for lock file", "lock file", "28 ")).toBe(true);
  });
});

describe("createSearchTracker", () => {
  test("reports the modal opening once per open", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("shortcut");
    tracker.open("shortcut");

    expect(captured).toEqual([
      { event: SEARCH_EVENTS.opened, properties: { trigger: "shortcut" } },
    ]);
  });

  test("reports the modal opening again after it closes", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.close();
    tracker.open("shortcut");

    expect(captured.map((c) => c.properties.trigger)).toEqual(["button", "shortcut"]);
  });

  test("reports a query with its result count", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("  run   queue ", 7);

    expect(captured[1]).toEqual({
      event: SEARCH_EVENTS.query,
      properties: { query: "run queue", query_length: 9, result_count: 7, has_results: true },
    });
  });

  test("marks a query that found nothing", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("nonexistent", 0);

    expect(captured[1]?.properties.has_results).toBe(false);
  });

  test("ignores an empty query", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("   ", 0);

    expect(captured).toHaveLength(1);
  });

  test("reports a repeated query only once", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stacks", 4);
    tracker.recordQuery("stacks", 4);

    expect(captured).toHaveLength(2);
  });

  test("reports a query the reader returns to after refining it", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stack", 9);
    tracker.recordQuery("stacks", 4);
    tracker.recordQuery("stack", 9);

    const queries = captured.filter((c) => c.event === SEARCH_EVENTS.query);
    expect(queries.map((c) => c.properties.query)).toEqual(["stack", "stacks", "stack"]);
  });

  test("reports a redacted query, measured at the length the reader typed", () => {
    const { tracker, captured } = makeTracker();
    const typed = "reset password=hunter2";

    tracker.open("button");
    tracker.recordQuery(typed, 3);

    expect(captured[1]?.properties).toEqual({
      query: "reset password=[redacted]",
      query_length: typed.length,
      result_count: 3,
      has_results: true,
    });
  });

  test("keeps the query redacted on the events that follow it", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("mail example@acme.com", 2);
    tracker.recordResultClick({ url: "/community/support/", position: 1, isSubResult: false });
    tracker.close();
    tracker.open("button");
    tracker.recordQuery("mail example@acme.com", 2);
    tracker.close();

    const carrying = captured.filter((c) => "query" in c.properties);
    expect(carrying.map((c) => c.event)).toEqual([
      SEARCH_EVENTS.query,
      SEARCH_EVENTS.resultClicked,
      SEARCH_EVENTS.query,
      SEARCH_EVENTS.abandoned,
    ]);
    expect(carrying.every((c) => c.properties.query === "mail [email]")).toBe(true);
  });

  test("attributes a result click to the query that produced it", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stacks", 4);
    tracker.recordResultClick({ url: "/features/stacks/", position: 2, isSubResult: true });

    expect(captured[2]).toEqual({
      event: SEARCH_EVENTS.resultClicked,
      properties: {
        query: "stacks",
        result_count: 4,
        url: "/features/stacks/",
        position: 2,
        is_sub_result: true,
      },
    });
  });

  test("reports an abandoned search when the modal closes without a click", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stacks", 4);
    tracker.close();

    expect(captured[2]).toEqual({
      event: SEARCH_EVENTS.abandoned,
      properties: { query: "stacks", query_length: 6, result_count: 4 },
    });
  });

  test("does not report an abandoned search after a result click", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stacks", 4);
    tracker.recordResultClick({ url: "/features/stacks/", position: 1, isSubResult: false });
    tracker.close();

    expect(captured.map((c) => c.event)).not.toContain(SEARCH_EVENTS.abandoned);
  });

  test("does not report an abandoned search when nothing was typed", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("shortcut");
    tracker.close();

    expect(captured).toHaveLength(1);
  });

  test("does not carry a query across two openings of the modal", () => {
    const { tracker, captured } = makeTracker();

    tracker.open("button");
    tracker.recordQuery("stacks", 4);
    tracker.close();
    tracker.open("button");
    tracker.close();

    expect(captured.filter((c) => c.event === SEARCH_EVENTS.abandoned)).toHaveLength(1);
  });
});

describe("kapaEventProperties", () => {
  test("converts payload keys to snake case", () => {
    const properties = kapaEventProperties({ threadId: "t-1", questionAnswerId: "qa-1" });

    expect(properties).toEqual({ thread_id: "t-1", question_answer_id: "qa-1" });
  });

  test("keeps numbers and booleans", () => {
    expect(kapaEventProperties({ resultCount: 3, isFirst: false })).toEqual({
      result_count: 3,
      is_first: false,
    });
  });

  test("replaces the answer with its length", () => {
    expect(kapaEventProperties({ answer: "a".repeat(42) })).toEqual({ answer_length: 42 });
  });

  test("drops conversation history", () => {
    const properties = kapaEventProperties({
      question: "what is a stack?",
      conversation: [{ role: "user", content: "what is a stack?" }],
      sources: ["/features/stacks/"],
    });

    expect(properties).toEqual({ question: "what is a stack?" });
  });

  test("redacts credentials out of the question", () => {
    expect(kapaEventProperties({ question: "why is AKIAIOSFODNN7EXAMPLE denied?" })).toEqual({
      question: "why is [redacted] denied?",
    });
  });

  test("truncates long strings", () => {
    const properties = kapaEventProperties({ question: "a".repeat(1500) });

    expect(properties.question).toHaveLength(1000);
  });

  test("drops values that are neither scalar nor a known bulky field", () => {
    expect(kapaEventProperties({ threadId: "t-1", metadata: { tier: "free" } })).toEqual({
      thread_id: "t-1",
    });
  });

  test("returns no properties for a payload that is not an object", () => {
    expect(kapaEventProperties(undefined)).toEqual({});
    expect(kapaEventProperties("stacks")).toEqual({});
  });
});
