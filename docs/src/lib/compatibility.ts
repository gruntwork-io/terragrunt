import type { CollectionEntry } from "astro:content";
import { isReleased } from "./changelog";

export type CompatibilityEntry = CollectionEntry<"compatibility">;

// Hide entries whose terragrunt_min has not been released yet.
export function filterByReleasedMin<T extends CompatibilityEntry>(
  entries: T[],
  latestVersion: string,
  showUnreleased = false,
): T[] {
  if (showUnreleased) return entries;
  return entries.filter((e) => isReleased(`v${e.data.terragrunt_min}`, latestVersion));
}

// Clear terragrunt_max when it points to a version that has not been released yet.
export function effectiveTerragruntMax(
  terragruntMax: string | null,
  latestVersion: string,
  showUnreleased = false,
): string | null {
  if (terragruntMax === null) return null;
  if (showUnreleased) return terragruntMax;
  if (!isReleased(`v${terragruntMax}`, latestVersion)) return null;
  return terragruntMax;
}
