// Markdown for docs pages and the pages that `.astro` routes render from data
// collections. Each page's `.md` route and the `llms-full.txt` and
// `llms-small.txt` routes call the same builders here.

import type { APIContext } from 'astro';
import { getCollection, getEntry, type CollectionEntry } from 'astro:content';
import ExperimentEntries from '@components/ExperimentEntries.astro';
import StrictControlEntries from '@components/StrictControlEntries.astro';
import { getPublishedCommands } from '@lib/commands/published';
import { sortFaqEntries, type FaqEntry } from '@lib/faq';
import { hasRenderableBody } from '@lib/docs-collection';
import { HCL_KINDS, referenceMarkdown, type HclEntry } from '@lib/hcl';
import { getPublishedHclItems } from '@lib/hcl/published';
import { markdownDocument, type MarkdownDetail, type MarkdownPage } from '@lib/markdown-document';
import {
	componentToMarkdown,
	entryToMarkdown,
	entryToMarkdownOrSource,
} from '@lib/page-to-markdown';
import { sortPatternEntries, type PatternEntry } from '@lib/patterns';
import { isPublished } from '@lib/release';

type CommandEntry = CollectionEntry<'commands'>;

export const ENTRY_STATUSES = ['active', 'completed'] as const;

export type EntryStatus = (typeof ENTRY_STATUSES)[number];

type PageMeta = Pick<MarkdownPage, 'title' | 'description'>;

const EXPERIMENT_META: Record<EntryStatus, PageMeta> = {
	active: {
		title: 'Active Experiments',
		description: 'Experimental features available for opt-in.',
	},
	completed: {
		title: 'Completed Experiments',
		description: 'Experiments that have graduated to stable features.',
	},
};

const STRICT_CONTROL_META: Record<EntryStatus, PageMeta> = {
	active: {
		title: 'Active Strict Controls',
		description: 'Strict controls that convert deprecated feature warnings into errors.',
	},
	completed: {
		title: 'Completed Strict Controls',
		description: 'Strict controls whose behaviour is now the default.',
	},
};

/** Builds a `.md` route's response for a page. */
export function markdownResponse({ title, description, body }: MarkdownPage): Response {
	return new Response(markdownDocument(title, description, body), {
		headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
	});
}

/** Returns the published `docs` entries, leaving out placeholders. */
export async function getRenderableDocs(): Promise<CollectionEntry<'docs'>[]> {
	return getCollection('docs', (doc) => !doc.data.draft && hasRenderableBody(doc.body));
}

/** Builds a page from a `docs` entry. */
export async function docsPage(
	entry: CollectionEntry<'docs'>,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	return {
		title: entry.data.hero?.title || entry.data.title,
		description: entry.data.hero?.tagline || entry.data.description,
		body: await entryToMarkdown(entry, context, detail),
	};
}

/**
 * Builds a CLI command page, mirroring the structure of `Command.astro`:
 * experiment note, usage, examples, body, then flags.
 *
 * `Command.astro` renders examples and one flag body with Expressive Code's
 * `<Code>` component, which can't run in the container used by the pipeline.
 * Examples are therefore fenced code blocks built from data, and bodies go
 * through `entryToMarkdownOrSource` so the `<Code>`-using flag doesn't fail the build.
 */
export async function commandPage(
	command: CommandEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
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
			sections.push('```' + (example.lang ?? 'bash') + '\n' + example.code.trim() + '\n```');
		}
	}

	const body = await entryToMarkdownOrSource(command, context, detail);
	if (body.trim()) sections.push(body);

	if (data.flags?.length) {
		const flagSections = (
			await Promise.all(data.flags.map((slug) => flagSection(slug, context, detail)))
		).filter((s): s is string => s !== null);
		if (flagSections.length) {
			sections.push('## Flags', ...flagSections);
		}
	}

	return { title: data.name, description: data.description, body: sections.join('\n\n') };
}

/**
 * Builds an HCL reference page, mirroring `HclBlock.astro`, `HclAttribute.astro`,
 * and `HclFunction.astro`: the structured fields that lead the page, the entry's
 * body, then the sections built from its fields.
 */
export async function hclPage(
	entry: HclEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	const { before, after } = referenceMarkdown(entry);
	const body = await entryToMarkdownOrSource(entry, context, detail);
	const sections = [before, body, after].filter((section) => section.trim());
	return { title: entry.data.name, description: entry.data.description, body: sections.join('\n\n') };
}

/** Builds the experiments page for a status. */
export async function experimentsPage(
	status: EntryStatus,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	// `showUnreleased` mirrors the `.astro` pages, which use `import.meta.env.DEV`
	// (i.e. false in a production build).
	const body = await componentToMarkdown(
		ExperimentEntries,
		{ status, showUnreleased: false },
		context,
		detail,
	);
	return { ...EXPERIMENT_META[status], body };
}

/** Builds the strict controls page for a status. */
export async function strictControlsPage(
	status: EntryStatus,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	const body = await componentToMarkdown(
		StrictControlEntries,
		{ status, showUnreleased: false },
		context,
		detail,
	);
	return { ...STRICT_CONTROL_META[status], body };
}

/** Builds the page for one FAQ question. */
export async function faqPage(
	entry: FaqEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	return {
		title: entry.data.question,
		description: entry.data.description,
		body: await entryToMarkdown(entry, context, detail),
	};
}

/** Builds the page for one pattern, opening with its author byline. */
export async function patternPage(
	entry: PatternEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<MarkdownPage> {
	const body = await entryToMarkdown(entry, context, detail);
	return {
		title: entry.data.title,
		description: entry.data.description,
		body: `_By ${entry.data.author}_\n\n${body}`,
	};
}

/** Returns the pages one data collection adds to the llms output. */
type LlmsPageSource = (context: APIContext, detail: MarkdownDetail) => Promise<MarkdownPage[]>;

/**
 * The pages in `llms-full.txt` and `llms-small.txt`, in output order. Append a
 * source here to add a page type.
 *
 * A source must return published pages only. Changelog releases are left out on
 * purpose: they describe past behavior that can contradict the current docs, and
 * each release stays available at its `.md` URL.
 *
 * Pages render one at a time: the container render is CPU-bound, so fanning the
 * whole collection out at once only raises peak memory.
 */
export const LLMS_PAGE_SOURCES: LlmsPageSource[] = [
	async (context, detail) => {
		const docs = await getRenderableDocs();
		docs.sort((a, b) => a.id.localeCompare(b.id, 'en'));
		return mapInOrder(docs, (entry) => docsPage(entry, context, detail));
	},
	async (context, detail) => {
		const entries = (await Promise.all(HCL_KINDS.map((kind) => getPublishedHclItems(kind)))).flat();
		return mapInOrder(entries, (entry) => hclPage(entry, context, detail));
	},
	async (context, detail) => {
		const commands = await getPublishedCommands();
		commands.sort((a, b) => a.data.path.localeCompare(b.data.path, 'en'));
		return mapInOrder(commands, (command) => commandPage(command, context, detail));
	},
	(context, detail) =>
		mapInOrder(ENTRY_STATUSES, (status) => experimentsPage(status, context, detail)),
	(context, detail) =>
		mapInOrder(ENTRY_STATUSES, (status) => strictControlsPage(status, context, detail)),
	async (context, detail) => {
		const entries = sortFaqEntries(await getCollection('faq'));
		return mapInOrder(entries, (entry) => faqPage(entry, context, detail));
	},
	async (context, detail) => {
		const entries = sortPatternEntries(await getCollection('patterns'));
		return mapInOrder(entries, (entry) => patternPage(entry, context, detail));
	},
];

async function mapInOrder<T, R>(
	items: readonly T[],
	build: (item: T) => Promise<R>,
): Promise<R[]> {
	const results: R[] = [];
	for (const item of items) {
		results.push(await build(item));
	}
	return results;
}

async function flagSection(
	slug: string,
	context: APIContext,
	detail: MarkdownDetail,
): Promise<string | null> {
	const flag = await getEntry('flags', slug);
	if (!flag) return null;
	if (!(await isPublished(flag.data.since))) return null;

	const { name, description, type, defaultVal, aliases, env } = flag.data;
	const parts = [`### \`--${name}\``];
	if (description) parts.push(description);

	const body = await entryToMarkdownOrSource(flag, context, detail);
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
