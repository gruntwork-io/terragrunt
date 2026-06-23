// Clean Markdown for each pattern page (`/guides/patterns/<slug>`), rendered by
// the sibling `[slug].astro` route from the `patterns` collection. Renders the
// pattern's body to Markdown, prefixed with the author byline.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { entryToSimpleMarkdown, markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

export const getStaticPaths = (async () => {
  const entries = await getCollection('patterns');
  return entries.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
  const { entry } = context.props as {
    entry: Awaited<ReturnType<typeof getCollection<'patterns'>>>[number];
  };

  const body = await entryToSimpleMarkdown(entry, context);
  const withByline = `_By ${entry.data.author}_\n\n${body}`;

  return new Response(
    markdownDocument(entry.data.title, entry.data.description, withByline),
    { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
  );
};
