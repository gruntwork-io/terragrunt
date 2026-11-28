// Clean Markdown for each pattern page (`/guides/patterns/<slug>`), rendered by
// the sibling `[slug].astro` route from the `patterns` collection. Renders the
// pattern's body to Markdown, prefixed with the author byline.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { markdownResponse, patternPage } from '@lib/generated-pages';
import type { PatternEntry } from '@lib/patterns';

export const prerender = true;

export const getStaticPaths = (async () => {
  const entries = await getCollection('patterns');
  return entries.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
  const { entry } = context.props as { entry: PatternEntry };
  return markdownResponse(await patternPage(entry, context));
};
