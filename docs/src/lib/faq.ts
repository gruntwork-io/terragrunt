import type { CollectionEntry } from "astro:content";

export type FaqEntry = CollectionEntry<"faq">;

/**
 * Sort FAQ entries for display: by the optional `order` field (ascending),
 * then alphabetically by question for entries that share (or omit) an order.
 * The entry `id` is the file's name without extension and serves as the URL
 * slug under `/process/faq/`.
 */
export function sortFaqEntries(entries: FaqEntry[]): FaqEntry[] {
  return [...entries].sort((a, b) => {
    const orderA = a.data.order ?? Number.POSITIVE_INFINITY;
    const orderB = b.data.order ?? Number.POSITIVE_INFINITY;
    if (orderA !== orderB) return orderA - orderB;
    return a.data.question.localeCompare(b.data.question);
  });
}
