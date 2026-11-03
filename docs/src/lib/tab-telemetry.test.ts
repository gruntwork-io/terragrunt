import { describe, expect, test } from "bun:test";
import { TAB_EVENTS, createTabTracker, labelForSlug, tabSlug, viewSource } from "./tab-telemetry";

const CLOUD_TABS = ["AWS", "Azure", "Google Cloud"];

interface CapturedEvent {
  event: string;
  properties: Record<string, unknown>;
}

function makeTracker() {
  const captured: CapturedEvent[] = [];
  const tracker = createTabTracker((event, properties) => {
    captured.push({ event, properties });
  });
  return { tracker, captured };
}

describe("tabSlug", () => {
  test("lowercases a single word", () => {
    expect(tabSlug("AWS")).toBe("aws");
  });

  test("joins words with a hyphen", () => {
    expect(tabSlug("Google Cloud")).toBe("google-cloud");
  });

  test("trims hyphens left by leading and trailing punctuation", () => {
    expect(tabSlug(" (Azure) ")).toBe("azure");
  });
});

describe("labelForSlug", () => {
  test("finds the label for a slug", () => {
    expect(labelForSlug(CLOUD_TABS, "google-cloud")).toBe("Google Cloud");
  });

  test("ignores case", () => {
    expect(labelForSlug(CLOUD_TABS, "AZURE")).toBe("Azure");
  });

  test("returns nothing for an unknown value", () => {
    expect(labelForSlug(CLOUD_TABS, "oracle")).toBeUndefined();
  });

  test("returns nothing for a value that names an object property", () => {
    expect(labelForSlug(CLOUD_TABS, "constructor")).toBeUndefined();
  });

  test("returns nothing when the link has no value", () => {
    expect(labelForSlug(CLOUD_TABS, null)).toBeUndefined();
  });
});

describe("viewSource", () => {
  test("credits the link when it selected the active tab", () => {
    expect(viewSource("Azure", "Azure", "Azure")).toBe("link");
  });

  test("credits the saved choice when there is no link", () => {
    expect(viewSource("Azure", undefined, "Azure")).toBe("saved");
  });

  test("credits the saved choice when the link names a tab this page lacks", () => {
    expect(viewSource("Azure", "Google Cloud", "Azure")).toBe("saved");
  });

  test("falls back to the default tab", () => {
    expect(viewSource("AWS", undefined, null)).toBe("default");
  });
});

describe("createTabTracker", () => {
  test("reports the first view of a sync key", () => {
    const { tracker, captured } = makeTracker();

    tracker.viewed("cloud-provider", "Azure", "link");

    expect(captured).toEqual([
      {
        event: TAB_EVENTS.viewed,
        properties: { sync_key: "cloud-provider", tab: "Azure", source: "link" },
      },
    ]);
  });

  test("reports one view per sync key on a page with several tab sets", () => {
    const { tracker, captured } = makeTracker();

    tracker.viewed("cloud-provider", "AWS", "default");
    tracker.viewed("cloud-provider", "AWS", "default");
    tracker.viewed("package-manager", "bun", "saved");

    expect(captured.map((c) => c.properties.sync_key)).toEqual(["cloud-provider", "package-manager"]);
  });

  test("reports a selection with the tab it replaced", () => {
    const { tracker, captured } = makeTracker();

    tracker.viewed("cloud-provider", "AWS", "default");
    tracker.selected("cloud-provider", "Azure");

    expect(captured[1]).toEqual({
      event: TAB_EVENTS.selected,
      properties: { sync_key: "cloud-provider", tab: "Azure", previous_tab: "AWS" },
    });
  });

  test("ignores a click that leaves the selection unchanged", () => {
    const { tracker, captured } = makeTracker();

    tracker.viewed("cloud-provider", "AWS", "default");
    tracker.selected("cloud-provider", "AWS");

    expect(captured).toHaveLength(1);
  });

  test("tracks the selection across repeated switches", () => {
    const { tracker, captured } = makeTracker();

    tracker.viewed("cloud-provider", "AWS", "default");
    tracker.selected("cloud-provider", "Azure");
    tracker.selected("cloud-provider", "AWS");

    expect(captured.map((c) => c.properties.previous_tab)).toEqual([undefined, "AWS", "Azure"]);
  });

  test("reports no previous tab for a selection before any view", () => {
    const { tracker, captured } = makeTracker();

    tracker.selected("cloud-provider", "Azure");

    expect(captured[0]?.properties.previous_tab).toBeNull();
  });
});
