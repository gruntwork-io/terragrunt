/**
 * Matches a Terragrunt release tag, e.g. `v1.2.0`. Every version the docs gate
 * content on uses this form: `since`, `completedSince`, and changelog
 * `version` frontmatter, and the `version` prop of `<Before>` and `<Since>`.
 */
export const RELEASE_VERSION = /^v\d+\.\d+\.\d+$/;

/**
 * Throws unless `version` has the `vX.Y.Z` form of {@link RELEASE_VERSION}.
 * Content collection schemas check frontmatter; this checks component props,
 * which no schema sees.
 */
export function assertReleaseVersion(version: string): void {
  if (!RELEASE_VERSION.test(version)) {
    throw new Error(`version "${version}" must have the form vX.Y.Z, e.g. v1.2.0`);
  }
}

/**
 * Returns true when `version` is at or below `latestVersion`, both in `vX.Y.Z`
 * form. Components compare numerically, so `v1.0.10` is newer than `v1.0.7`.
 */
export function isReleased(version: string, latestVersion: string): boolean {
  return version.localeCompare(latestVersion, undefined, { numeric: true }) <= 0;
}

/**
 * Sorts `vX.Y.Z` versions newest first.
 */
export function compareVersionsDesc(a: string, b: string): number {
  return b.localeCompare(a, undefined, { numeric: true });
}
