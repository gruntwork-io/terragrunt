import type { CaptureFn } from "./search-telemetry";

export const TAB_EVENTS = {
  viewed: "docs_tabs_viewed",
  selected: "docs_tab_selected",
} as const;

/**
 * How the tab shown on page load got selected: a query parameter in the link the reader followed,
 * a choice the reader made earlier and Starlight saved, or the first tab.
 */
export type TabSource = "link" | "saved" | "default";

export interface TabTracker {
  viewed(syncKey: string, tab: string, source: TabSource): void;
  selected(syncKey: string, tab: string): void;
}

/**
 * Turns a tab label into its slug: lowercased, with runs of other characters replaced by `-`.
 * "AWS" becomes "aws" and "Google Cloud" becomes "google-cloud".
 */
export function tabSlug(label: string): string {
  return label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

/** Finds the label whose slug matches `value`, ignoring case. */
export function labelForSlug(labels: readonly string[], value: string | null): string | undefined {
  if (!value) return undefined;

  const wanted = value.toLowerCase();

  return labels.find((label) => tabSlug(label) === wanted);
}

/** Works out how `active` came to be the selected tab on page load. */
export function viewSource(active: string, linked: string | undefined, saved: string | null): TabSource {
  if (linked === active) return "link";
  if (saved === active) return "saved";

  return "default";
}

/**
 * Reports which tab readers see and which tabs they pick, once per sync key.
 *
 * Synced tab sets on a page switch together, so a page with ten sets reports one view, and a click
 * that leaves the selection where it was reports nothing.
 */
export function createTabTracker(capture: CaptureFn): TabTracker {
  const current = new Map<string, string>();

  return {
    viewed(syncKey, tab, source) {
      if (current.has(syncKey)) return;

      current.set(syncKey, tab);
      capture(TAB_EVENTS.viewed, { sync_key: syncKey, tab, source });
    },

    selected(syncKey, tab) {
      const previous = current.get(syncKey);
      if (previous === tab) return;

      current.set(syncKey, tab);
      capture(TAB_EVENTS.selected, { sync_key: syncKey, tab, previous_tab: previous ?? null });
    },
  };
}
