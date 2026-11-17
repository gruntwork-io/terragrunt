// Clean Markdown for the dynamically-generated CLI command reference pages.
//
// The HTML pages at `/reference/cli/commands/<path>/` are rendered by the
// sibling `[...slug].astro` route from the `commands` collection; the matching
// `docs` collection entries are intentionally-empty placeholders.

import type { APIRoute, GetStaticPaths } from 'astro';
import type { CollectionEntry } from 'astro:content';
import { getPublishedCommands } from '@lib/commands/published';
import { commandPage, markdownResponse } from '@lib/generated-pages';

export const prerender = true;

export const getStaticPaths = (async () => {
	const commands = await getPublishedCommands();
	return commands.map((command) => ({
		// `data.path` is the slug the HTML route uses, so `<path>.md` mirrors it.
		params: { slug: command.data.path },
		props: { command },
	}));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { command } = context.props as { command: CollectionEntry<'commands'> };
	return markdownResponse(await commandPage(command, context));
};
