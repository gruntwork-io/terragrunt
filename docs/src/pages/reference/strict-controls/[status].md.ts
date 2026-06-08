// Clean Markdown for the generated strict-controls reference pages
// (`/reference/strict-controls/active` and `/completed`), rendered by the
// sibling `.astro` routes from the `strictControls` collection. Renders the same
// `StrictControlEntries` component to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import StrictControlEntries from '@components/StrictControlEntries.astro';
import { componentToSimpleMarkdown, markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

const META = {
	active: {
		title: 'Active Strict Controls',
		description:
			'Strict controls that convert deprecated feature warnings into errors.',
	},
	completed: {
		title: 'Completed Strict Controls',
		description: 'Strict controls whose behaviour is now the default.',
	},
} as const;

export const getStaticPaths = (() =>
	(Object.keys(META) as (keyof typeof META)[]).map((status) => ({
		params: { status },
	}))) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const status = context.params.status as keyof typeof META;
	const meta = META[status];
	if (!meta) return new Response('Not found', { status: 404 });

	const body = await componentToSimpleMarkdown(
		StrictControlEntries,
		{ status, showUnreleased: false },
		context,
	);

	return new Response(markdownDocument(meta.title, meta.description, body), {
		headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
	});
};
