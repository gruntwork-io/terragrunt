import type { CollectionEntry } from "astro:content";

export type ChangelogEntry = CollectionEntry<"changelog">;

export interface ChangelogCategory {
  slug: string;
  label: string;
}

export const CHANGELOG_CATEGORIES: readonly ChangelogCategory[] = [
  { slug: "breaking-changes", label: "🛠️ Breaking Changes" },
  { slug: "new-features", label: "✨ New Features" },
  { slug: "performance-improvements", label: "🏎️ Performance Improvements" },
  { slug: "tips-added", label: "💡 Tips Added" },
  { slug: "bug-fixes", label: "🐛 Bug Fixes" },
  { slug: "documentation-updates", label: "📖 Documentation Updates" },
  { slug: "experiments-added", label: "🧪 Experiments Added" },
  { slug: "experiments-updated", label: "🧪 Experiments Updated" },
  { slug: "process-updates", label: "⚙️ Process Updates" },
] as const;

export const CHANGELOG_CATEGORY_SLUGS = CHANGELOG_CATEGORIES.map((c) => c.slug) as [
  string,
  ...string[],
];

const CATEGORY_INDEX = new Map(CHANGELOG_CATEGORIES.map((c, i) => [c.slug, i]));

export function categoryLabel(slug: string): string {
  return CHANGELOG_CATEGORIES.find((c) => c.slug === slug)?.label ?? slug;
}

export function compareVersionsDesc(a: string, b: string): number {
  const aIsVersion = a.startsWith("v");
  const bIsVersion = b.startsWith("v");
  if (!aIsVersion && bIsVersion) return -1;
  if (aIsVersion && !bIsVersion) return 1;
  return b.localeCompare(a, undefined, { numeric: true });
}

export function isReleased(version: string, latestVersion: string): boolean {
  if (!version.startsWith("v")) return false;
  const v = version.replace(/^v/, "");
  return v.localeCompare(latestVersion, undefined, { numeric: true }) <= 0;
}

export interface CategoryGroup {
  category: ChangelogCategory;
  entries: ChangelogEntry[];
}

export function groupByCategory(entries: ChangelogEntry[]): CategoryGroup[] {
  const buckets = new Map<string, ChangelogEntry[]>();
  for (const entry of entries) {
    const slug = entry.data.category;
    const bucket = buckets.get(slug) ?? [];
    bucket.push(entry);
    buckets.set(slug, bucket);
  }

  for (const bucket of buckets.values()) {
    bucket.sort((a, b) => {
      const orderA = a.data.order ?? Number.POSITIVE_INFINITY;
      const orderB = b.data.order ?? Number.POSITIVE_INFINITY;
      if (orderA !== orderB) return orderA - orderB;
      return a.id.localeCompare(b.id);
    });
  }

  return CHANGELOG_CATEGORIES.filter((c) => buckets.has(c.slug)).map((category) => ({
    category,
    entries: buckets.get(category.slug)!,
  }));
}

export function uniqueVersions(entries: ChangelogEntry[]): string[] {
  const set = new Set<string>();
  for (const entry of entries) set.add(entry.data.version);
  return Array.from(set).sort(compareVersionsDesc);
}

export function entriesForVersion(
  entries: ChangelogEntry[],
  version: string,
): ChangelogEntry[] {
  return entries.filter((e) => e.data.version === version);
}

export function categorySlugSort(a: string, b: string): number {
  const ai = CATEGORY_INDEX.get(a) ?? Number.POSITIVE_INFINITY;
  const bi = CATEGORY_INDEX.get(b) ?? Number.POSITIVE_INFINITY;
  return ai - bi;
}

export function prepareForGitHub(body: string, siteUrl: string): string {
  let result = body;

  result = result.replace(/\]\(\//g, `](${siteUrl}/`);

  result = result.replace(/^####\s/gm, "### ");
  result = result.replace(/^###\s/gm, "## ");

  result = result.replace(/^\s*---\s*$/gm, "");

  result = result.replace(/\n{3,}/g, "\n\n");

  return result.trim();
}
