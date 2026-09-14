import { defineCollection } from "astro:content";
import { z } from "astro/zod";
import { glob } from "astro/loaders";

/**
 * The filename is the route, in all three collections. There is deliberately no
 * `slug` field: Astro's glob loader would take one as the entry id, which makes
 * it load-bearing, and a second copy of the id is a second thing that can drift
 * away from the url a crawler already knows.
 */

const faq = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "src/content/faq" }),
  schema: z.object({
    /** Row title on `/faqs-overview` and the `<h1>` on the question's own page. */
    question: z.string(),
    title: z.string(),
    description: z.string(),
    /** Which accordion on `/faqs-overview` the entry belongs to. */
    category: z.enum(["terragrunt", "terragrunt-scale"]),
    /** Position within that accordion. Lower sorts first. */
    order: z.number(),
  }),
});

const comparisons = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "src/content/comparisons" }),
  schema: z.object({
    headline: z.string(),
    /** Card label on the index, which differs from the page's own headline. */
    indexTitle: z.string(),
    eyebrow: z.string().default("Comparison"),
    subhead: z.string().optional(),
    title: z.string(),
    description: z.string(),
    /** Standfirst above the body, styled as a lede rather than body copy. */
    lede: z.string().optional(),
    order: z.number().optional(),
  }),
});

const guides = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "src/content/guides" }),
  schema: z.object({
    headline: z.string(),
    indexTitle: z.string(),
    eyebrow: z.string().default("Guide"),
    subhead: z.string().optional(),
    title: z.string(),
    description: z.string(),
    lede: z.string().optional(),
    order: z.number().optional(),
  }),
});

export const collections = { comparisons, faq, guides };
