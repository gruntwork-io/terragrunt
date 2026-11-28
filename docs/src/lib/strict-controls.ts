import type { CollectionEntry } from "astro:content";
import { isReleased } from "./versions";

export type StrictControlEntry = CollectionEntry<"strictControls">;

/**
 * Returns true once the release in which the strict control concluded has
 * shipped. Compares against the real latest release rather than honoring the
 * dev-only `showUnreleased` override, so the pre-completion notice stays
 * visible while authoring locally.
 */
export function isCompleted(entry: StrictControlEntry, latestVersion: string): boolean {
  const { completedSince } = entry.data;
  return completedSince !== undefined && isReleased(completedSince, latestVersion);
}
