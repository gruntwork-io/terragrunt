import type { CollectionEntry } from "astro:content";

export type ExperimentEntry = CollectionEntry<"experiments">;

/**
 * Returns true when the latest release is equal to or newer than the given
 * version. An undefined version is treated as released, matching the no-value
 * case of the `since` field.
 */
export function isVersionReleased(version: string | undefined, latestVersion: string): boolean {
  if (!version) return true;
  const target = version.replace(/^v/, "");
  return latestVersion.localeCompare(target, undefined, { numeric: true }) >= 0;
}

/**
 * Returns true once the release in which the experiment concluded has shipped.
 * An experiment completes whether it graduated to a default feature or was
 * retired, so completion is the umbrella concept and there is no separate
 * status field. Compares against the real latest release rather than honoring
 * the dev-only `showUnreleased` override, so the pre-completion notice stays
 * visible while authoring locally.
 */
export function isCompleted(entry: ExperimentEntry, latestVersion: string): boolean {
  return Boolean(entry.data.completedSince)
    && isVersionReleased(entry.data.completedSince, latestVersion);
}
