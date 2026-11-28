// Clean Markdown for the pages of variable fields rendered by the sibling
// `[field].astro` route from the `hclVariableFields` collection.

import type { APIRoute, GetStaticPaths } from 'astro';
import { markdownResponse, variableFieldPage } from '@lib/generated-pages';
import type { VariableField } from '@lib/hcl';
import { getPublishedVariableFields } from '@lib/hcl/published';

export const prerender = true;

export const getStaticPaths = (async () =>
	(await getPublishedVariableFields()).map((entry) => ({
		params: { variable: entry.data.variable, field: entry.data.name },
		props: { entry },
	}))) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { entry } = context.props as { entry: VariableField };
	return markdownResponse(await variableFieldPage(entry, context));
};
