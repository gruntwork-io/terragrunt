// Clean Markdown for each FAQ question page (`/process/faq/<slug>`), rendered
// by the sibling `[slug].astro` route from the `faq` collection. Renders the
// question's answer body to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { entryToSimpleMarkdown, markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

export const getStaticPaths = (async () => {
  const entries = await getCollection('faq');
  return entries.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
  const { entry } = context.props as {
    entry: Awaited<ReturnType<typeof getCollection<'faq'>>>[number];
  };

  const body = await entryToSimpleMarkdown(entry, context);

  return new Response(
    markdownDocument(entry.data.question, entry.data.description, body),
    { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
  );
};
