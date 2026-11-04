// Clean Markdown for the HCL reference pages rendered by the sibling
// `[name].astro` route. Mirrors `HclBlock.astro`, `HclAttribute.astro`, and
// `HclFunction.astro`: the structured fields that lead the page, the entry's
// body, then the sections built from its fields.

import type { APIRoute, GetStaticPaths } from 'astro';
import { HCL_KINDS, referenceMarkdown, type HclEntry } from '@lib/hcl';
import { getPublishedHclItems } from '@lib/hcl/published';
import { markdownDocument, safeEntryToMarkdown } from '@lib/page-to-markdown';

export const prerender = true;

export const getStaticPaths = (async () => {
	const paths = await Promise.all(
		HCL_KINDS.map(async (kind) =>
			(await getPublishedHclItems(kind)).map((entry) => ({
				params: { kind, name: entry.data.name },
				props: { entry },
			})),
		),
	);
	return paths.flat();
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { entry } = context.props as { entry: HclEntry };
	const { before, after } = referenceMarkdown(entry);
	const body = await safeEntryToMarkdown(entry, context);
	const sections = [before, body, after].filter((section) => section.trim());

	return new Response(markdownDocument(entry.data.name, entry.data.description, sections.join('\n\n')), {
		headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
	});
};
