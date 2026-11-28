// Clean Markdown for the generated experiments reference pages
// (`/reference/experiments/active` and `/reference/experiments/completed`),
// which are rendered by the sibling `.astro` routes from the `experiments`
// collection. Renders the same `ExperimentEntries` component to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import {
	ENTRY_STATUSES,
	experimentsPage,
	markdownResponse,
	type EntryStatus,
} from '@lib/generated-pages';

export const prerender = true;

export const getStaticPaths = (() =>
	ENTRY_STATUSES.map((status) => ({ params: { status } }))) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) =>
	markdownResponse(await experimentsPage(context.params.status as EntryStatus, context));
