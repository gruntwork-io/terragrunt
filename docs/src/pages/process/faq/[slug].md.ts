// Clean Markdown for each FAQ question page (`/process/faq/<slug>`), rendered
// by the sibling `[slug].astro` route from the `faq` collection. Renders the
// question's answer body to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import type { FaqEntry } from '@lib/faq';
import { faqPage, markdownResponse } from '@lib/generated-pages';

export const prerender = true;

export const getStaticPaths = (async () => {
  const entries = await getCollection('faq');
  return entries.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
  const { entry } = context.props as { entry: FaqEntry };
  return markdownResponse(await faqPage(entry, context));
};
