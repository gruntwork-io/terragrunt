// Serves a clean Markdown version of every prose docs page at `<url>.md`.
//
// For a page rendered at `/getting-started/install/`, this emits a static
// `/getting-started/install.md` containing the same content converted to plain
// Markdown (MDX components resolved). This is the AEO ".md suffix" pattern
// popularised by Stripe's docs, letting LLMs and agents fetch a token-cheap,
// chrome-free version of any page.
//
// Prose docs live in the `docs` content collection and are handled here. The
// dynamically-generated reference pages (CLI commands, changelog, experiments,
// strict-controls) are `.astro` routes and get their own `.md` endpoints.

import type { APIRoute, GetStaticPaths } from 'astro';
import type { CollectionEntry } from 'astro:content';
import { docsPage, getRenderableDocs, markdownResponse } from '@lib/generated-pages';

export const prerender = true;

export const getStaticPaths = (async () => {
	const docs = await getRenderableDocs();
	return docs.map((entry) => ({
		// `entry.id` is the page's routing slug (frontmatter `slug`, numeric
		// path prefixes stripped), so `<slug>.md` mirrors the HTML URL exactly.
		params: { slug: entry.id },
		props: { entry },
	}));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { entry } = context.props as { entry: CollectionEntry<'docs'> };
	return markdownResponse(await docsPage(entry, context));
};
