// Clean Markdown for the HCL reference pages rendered by the sibling
// `[name].astro` route from the `hclBlocks`, `hclAttributes`, and
// `hclFunctions` collections.

import type { APIRoute, GetStaticPaths } from 'astro';
import { hclPage, markdownResponse } from '@lib/generated-pages';
import { HCL_KINDS, type HclEntry } from '@lib/hcl';
import { getPublishedHclItems } from '@lib/hcl/published';

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
	return markdownResponse(await hclPage(entry, context));
};
