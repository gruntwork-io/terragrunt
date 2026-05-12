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

export interface ConventionalType {
  key: string;
  label: string;
}

export const CONVENTIONAL_TYPES: readonly ConventionalType[] = [
  { key: "breaking", label: "🛠️ Breaking Changes" },
  { key: "feat", label: "✨ Features" },
  { key: "fix", label: "🐛 Bug Fixes" },
  { key: "perf", label: "🏎️ Performance" },
  { key: "refactor", label: "♻️ Refactors" },
  { key: "revert", label: "⏪ Reverts" },
  { key: "docs", label: "📖 Documentation" },
  { key: "test", label: "✅ Tests" },
  { key: "build", label: "📦 Build" },
  { key: "ci", label: "🤖 CI" },
  { key: "chore", label: "🧹 Chores" },
  { key: "style", label: "🎨 Style" },
  { key: "other", label: "📝 Other Changes" },
] as const;

const CONVENTIONAL_TYPE_KEYS = new Set(CONVENTIONAL_TYPES.map((t) => t.key));

const CONVENTIONAL_TYPE_ALIASES: Record<string, string> = {
  bug: "fix",
  doc: "docs",
};

export interface PullRequestItem {
  title: string;
  author: string;
  prUrl: string;
  prNumber: number | null;
  type: string;
}

export interface PullRequestGroup {
  type: ConventionalType;
  items: PullRequestItem[];
}

const PULL_REQUEST_LINE = /^\*\s+(.+?)\s+by\s+@([\w-]+)\s+in\s+(\S+?)\/?$/;
const PR_NUMBER = /\/pull\/(\d+)/;
const CONVENTIONAL_PREFIX = /^([a-z]+)(\([^)]*\))?(!)?:\s*(.*)$/i;
// Matches the heading GitHub auto-generates in release bodies; the literal
// text "What's Changed" is GitHub's convention and must stay verbatim.
const SECTION_HEADING = /^##\s+What's Changed\s*$/i;
const ANY_H2 = /^##\s+/;
const FULL_CHANGELOG = /^\*\*Full Changelog\*\*/i;
const BREAKING_CHANGE = /BREAKING CHANGE:/i;

function classifyTitle(title: string): string {
  if (BREAKING_CHANGE.test(title)) return "breaking";
  const match = CONVENTIONAL_PREFIX.exec(title);
  if (match === null) return "other";
  const lower = match[1].toLowerCase();
  const bang = match[3];
  if (bang === "!") return "breaking";
  const type = CONVENTIONAL_TYPE_ALIASES[lower] ?? lower;
  return CONVENTIONAL_TYPE_KEYS.has(type) ? type : "other";
}

function parsePullRequestLine(line: string): PullRequestItem | null {
  const match = PULL_REQUEST_LINE.exec(line.trim());
  if (match === null) return null;
  const [, title, author, prUrl] = match;
  const prMatch = PR_NUMBER.exec(prUrl);
  return {
    title,
    author,
    prUrl,
    prNumber: prMatch ? Number(prMatch[1]) : null,
    type: classifyTitle(title),
  };
}

function extractPullRequestItems(body: string): PullRequestItem[] {
  const items: PullRequestItem[] = [];
  let inSection = false;
  for (const line of body.split(/\r?\n/)) {
    if (SECTION_HEADING.test(line)) {
      inSection = true;
      continue;
    }
    if (!inSection) continue;
    if (ANY_H2.test(line) || FULL_CHANGELOG.test(line)) break;
    const item = parsePullRequestLine(line);
    if (item) items.push(item);
  }
  return items;
}

function groupItemsByType(items: PullRequestItem[]): PullRequestGroup[] {
  const buckets = new Map<string, PullRequestItem[]>();
  for (const item of items) {
    const bucket = buckets.get(item.type) ?? [];
    bucket.push(item);
    buckets.set(item.type, bucket);
  }
  const groups: PullRequestGroup[] = [];
  for (const type of CONVENTIONAL_TYPES) {
    const bucket = buckets.get(type.key);
    if (bucket) groups.push({ type, items: bucket });
  }
  return groups;
}

export function parsePullRequests(body: string | null | undefined): PullRequestGroup[] {
  if (!body) return [];
  const items = extractPullRequestItems(body);
  if (items.length === 0) return [];
  return groupItemsByType(items);
}

export function pullRequestsToMarkdown(groups: PullRequestGroup[]): string {
  if (groups.length === 0) return "";
  const sections = groups.map((group) => {
    const lines = group.items.map((item) => {
      const prRef = item.prNumber === null ? ` in ${item.prUrl}` : ` in [#${item.prNumber}](${item.prUrl})`;
      return `- ${item.title} by [@${item.author}](https://github.com/${item.author})${prRef}`;
    });
    return `### ${group.type.label}\n\n${lines.join("\n")}`;
  });
  return `## Pull Requests\n\n${sections.join("\n\n")}`;
}

// Maps Starlight `<Aside type="...">` values to the closest GitHub Flavored
// Markdown alert label. GitHub renders these as colored callout boxes in
// release notes and READMEs.
const ASIDE_TYPE_TO_GH_ALERT: Record<string, string> = {
  note: "NOTE",
  tip: "TIP",
  caution: "WARNING",
  danger: "CAUTION",
};

const ASIDE_BLOCK = /<Aside(?:\s+([^>]*?))?\s*>([\s\S]*?)<\/Aside>/g;
const ASIDE_TYPE_ATTR = /\btype\s*=\s*"([^"]+)"/;
const ASIDE_TITLE_ATTR = /\btitle\s*=\s*"([^"]+)"/;
const MDX_IMPORT_LINE = /^\s*import\s+[^\n]*\s+from\s+['"][^'"]+['"];?\s*$/gm;

function attrValue(attrs: string, pattern: RegExp): string | null {
  const match = attrs.match(pattern);
  return match ? match[1] : null;
}

function transformAsidesToGitHubAlerts(input: string): string {
  return input.replace(ASIDE_BLOCK, (_match, attrs: string | undefined, content: string) => {
    const attrString = attrs ?? "";
    const type = attrValue(attrString, ASIDE_TYPE_ATTR) ?? "note";
    const title = attrValue(attrString, ASIDE_TITLE_ATTR) ?? "";
    const alert = ASIDE_TYPE_TO_GH_ALERT[type] ?? "NOTE";

    const titleSection = title ? `**${title}**\n\n` : "";
    const body = `[!${alert}]\n${titleSection}${content.trim()}`;

    return body
      .split("\n")
      .map((line) => (line.length === 0 ? ">" : `> ${line}`))
      .join("\n");
  });
}

export function prepareForGitHub(body: string, siteUrl: string): string {
  let result = body;

  result = result.replace(MDX_IMPORT_LINE, "");

  result = transformAsidesToGitHubAlerts(result);

  result = result.replace(/\]\(\//g, `](${siteUrl}/`);

  result = result.replace(/^####\s/gm, "### ");
  result = result.replace(/^###\s/gm, "## ");

  result = result.replace(/^\s*---\s*$/gm, "");

  result = result.replace(/\n{3,}/g, "\n\n");

  return result.trim();
}
