// Clean Markdown for the generated experiments reference pages
// (`/reference/experiments/active` and `/reference/experiments/completed`),
// which are rendered by the sibling `.astro` routes from the `experiments`
// collection. Renders the same `ExperimentEntries` component to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import ExperimentEntries from '@components/ExperimentEntries.astro';
import { componentToSimpleMarkdown, markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

const META = {
	active: {
		title: 'Active Experiments',
		description: 'Experimental features available for opt-in.',
	},
	completed: {
		title: 'Completed Experiments',
		description: 'Experiments that have graduated to stable features.',
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

	// `showUnreleased` mirrors the `.astro` pages, which use `import.meta.env.DEV`
	// (i.e. false in a production build).
	const body = await componentToSimpleMarkdown(
		ExperimentEntries,
		{ status, showUnreleased: false },
		context,
	);

	return new Response(markdownDocument(meta.title, meta.description, body), {
		headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
	});
};
