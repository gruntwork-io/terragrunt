// Clean Markdown for the generated strict-controls reference pages
// (`/reference/strict-controls/active` and `/completed`), rendered by the
// sibling `.astro` routes from the `strictControls` collection. Renders the same
// `StrictControlEntries` component to Markdown.

import type { APIRoute, GetStaticPaths } from 'astro';
import {
	ENTRY_STATUSES,
	markdownResponse,
	strictControlsPage,
	type EntryStatus,
} from '@lib/generated-pages';

export const prerender = true;

export const getStaticPaths = (() =>
	ENTRY_STATUSES.map((status) => ({ params: { status } }))) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) =>
	markdownResponse(await strictControlsPage(context.params.status as EntryStatus, context));
