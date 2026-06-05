// Clean Markdown for the dynamically-generated CLI command reference pages.
//
// The HTML pages at `/reference/cli/commands/<path>/` are rendered by the
// sibling `[...slug].astro` route from the `commands` collection; the matching
// `docs` collection entries are intentionally-empty placeholders. We mirror the
// structure of `Command.astro` here (experiment note → Usage → Examples → body
// → Flags), building the structured parts from collection data and rendering the
// command/flag MDX bodies through the Markdown pipeline.
//
// Note: `Command.astro` renders examples and one flag body with Expressive
// Code's `<Code>` component, which can't run in the container used by the
// pipeline. We therefore emit examples as fenced code blocks from data, and use
// `safeEntryToMarkdown` for bodies so the rare `<Code>`-using flag degrades
// gracefully instead of failing the build.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection, getEntry } from 'astro:content';
import { isFlagVisible } from '@lib/flags';
import {
	markdownDocument,
	safeEntryToMarkdown,
} from '@lib/page-to-markdown';

export const prerender = true;

export const getStaticPaths = (async () => {
	const commands = await getCollection('commands');
	return commands.map((command) => ({
		// `data.path` is the slug the HTML route uses, so `<path>.md` mirrors it.
		params: { slug: command.data.path },
		props: { command },
	}));
}) satisfies GetStaticPaths;

type CommandEntry = Awaited<ReturnType<typeof getCollection<'commands'>>>[number];

async function flagToMarkdown(
	slug: string,
	context: Parameters<APIRoute>[0],
): Promise<string | null> {
	const flag = await getEntry('flags', slug);
	if (!flag) return null;
	if (!(await isFlagVisible(flag.data.since))) return null;

	const { name, description, type, defaultVal, aliases, env } = flag.data;
	const parts = [`### \`--${name}\``];
	if (description) parts.push(description);

	const body = await safeEntryToMarkdown(flag, context);
	if (body.trim()) parts.push(body);

	const meta: string[] = [`- **Type:** \`${type}\``];
	if (defaultVal) meta.push(`- **Default:** \`${defaultVal}\``);
	if (aliases?.length) meta.push(`- **Aliases:** ${aliases.map((a) => `\`${a}\``).join(', ')}`);
	if (env?.length) {
		meta.push(`- **Environment variables:** ${env.map((e) => `\`${e}\``).join(', ')}`);
	}
	parts.push(meta.join('\n'));

	return parts.join('\n\n');
}

async function commandToMarkdown(
	command: CommandEntry,
	context: Parameters<APIRoute>[0],
): Promise<string> {
	const data = command.data;
	const sections: string[] = [];

	if (data.experiment) {
		sections.push(
			`> **Experimental:** the \`${data.name}\` command requires the ` +
				`\`--experiment ${data.experiment.control}\` flag ` +
				`([${data.experiment.name}](/reference/experiments/active#${data.experiment.control})).`,
		);
	}

	if (data.usage) {
		sections.push('## Usage', data.usage.trim());
	}

	if (data.examples?.length) {
		sections.push('## Examples');
		for (const example of data.examples) {
			if (example.description) sections.push(example.description.trim());
			sections.push('```bash\n' + example.code.trim() + '\n```');
		}
	}

	const body = await safeEntryToMarkdown(command, context);
	if (body.trim()) sections.push(body);

	if (data.flags?.length) {
		const flagSections = (
			await Promise.all(data.flags.map((slug) => flagToMarkdown(slug, context)))
		).filter((s): s is string => s !== null);
		if (flagSections.length) {
			sections.push('## Flags', ...flagSections);
		}
	}

	return sections.join('\n\n');
}

export const GET: APIRoute = async (context) => {
	const { command } = context.props as { command: CommandEntry };
	const body = await commandToMarkdown(command, context);
	return new Response(
		markdownDocument(command.data.name, command.data.description, body),
		{ headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
	);
};
