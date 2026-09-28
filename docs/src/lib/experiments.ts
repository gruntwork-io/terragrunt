import type { CollectionEntry } from "astro:content";
import { isReleased } from "./versions";

export type ExperimentEntry = CollectionEntry<"experiments">;

/**
 * Returns true once the release in which the experiment concluded has shipped.
 * An experiment completes whether it graduated to a default feature or was
 * retired, so completion is the umbrella concept and there is no separate
 * status field. Compares against the real latest release rather than honoring
 * the dev-only `showUnreleased` override, so the pre-completion notice stays
 * visible while authoring locally.
 */
export function isCompleted(entry: ExperimentEntry, latestVersion: string): boolean {
  const { completedSince } = entry.data;
  return completedSince !== undefined && isReleased(completedSince, latestVersion);
}
