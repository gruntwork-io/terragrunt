import type { CollectionEntry } from "astro:content";

export type PatternEntry = CollectionEntry<"patterns">;

/**
 * Sort pattern entries for display: by the optional `order` field (ascending),
 * then alphabetically by title for entries that share (or omit) an order. The
 * entry `id` is the file's name without extension and serves as the URL slug
 * under `/guides/patterns/`.
 */
export function sortPatternEntries(entries: PatternEntry[]): PatternEntry[] {
  return [...entries].sort((a, b) => {
    const orderA = a.data.order ?? Number.POSITIVE_INFINITY;
    const orderB = b.data.order ?? Number.POSITIVE_INFINITY;
    if (orderA !== orderB) return orderA - orderB;
    return a.data.title.localeCompare(b.data.title);
  });
}
