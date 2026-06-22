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
import { getCollection } from 'astro:content';
import { entryToSimpleMarkdown } from '@lib/page-to-markdown';

export const prerender = true;

/**
 * Some docs entries are intentionally-empty placeholders whose visible content
 * is rendered by a dedicated `.astro` route (e.g. the `reference/cli/commands/*`
 * pages, generated from the `commands` collection, and the `process/changelog`
 * overview). Their markdown body is just an HTML comment, so rendering them here
 * would emit a near-empty `.md`. Skip them — their real `.md` is produced by the
 * matching generated-page endpoints instead.
 */
function hasRenderableBody(body: string | undefined): boolean {
	if (!body) return false;
	const stripped = body
		.replace(/<!--[\s\S]*?-->/g, '') // HTML comments (used by CLI command stubs)
		.replace(/\{\/\*[\s\S]*?\*\/\}/g, '') // MDX/JSX comments (used by the changelog stub)
		.trim();
	return stripped.length > 0;
}

export const getStaticPaths = (async () => {
	const docs = await getCollection('docs', (doc) => !doc.data.draft);
	return docs
		.filter((entry) => hasRenderableBody(entry.body))
		.map((entry) => ({
			// `entry.id` is the page's routing slug (frontmatter `slug`, numeric
			// path prefixes stripped), so `<slug>.md` mirrors the HTML URL exactly.
			params: { slug: entry.id },
			props: { entry },
		}));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { entry } = context.props as {
		entry: Awaited<ReturnType<typeof getCollection<'docs'>>>[number];
	};

	const title = entry.data.hero?.title || entry.data.title;
	const description = entry.data.hero?.tagline || entry.data.description;

	const body = await entryToSimpleMarkdown(entry, context, false);

	const segments = [`# ${title}`];
	if (description) segments.push(`> ${description}`);
	segments.push(body);

	return new Response(segments.join('\n\n') + '\n', {
		headers: {
			'Content-Type': 'text/markdown; charset=utf-8',
		},
	});
};
