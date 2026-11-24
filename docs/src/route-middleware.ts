import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { getEntry } from 'astro:content';
import { getPublishedCommands } from '@lib/commands/published';
import { HCL_KINDS, hclItemPath, type HclKind } from '@lib/hcl';
import { getPublishedHclItems, isExperimental } from '@lib/hcl/published';
import { addDataPages, type DataPage } from '@lib/sidebar';

const HCL_GROUP = ['Reference', 'HCL'];

// The sidebar config has a group for each kind but variables, whose overview
// page is gated on `since` and can't be listed there before its release.
const HCL_GROUPS: Record<HclKind, Pick<DataPage, 'group' | 'subgroup'>> = {
	blocks: { group: [...HCL_GROUP, 'Blocks'] },
	attributes: { group: [...HCL_GROUP, 'Attributes'] },
	functions: { group: [...HCL_GROUP, 'Functions'] },
	variables: { group: HCL_GROUP, subgroup: 'Variables' },
};

const VARIABLES_SLUG = 'reference/hcl/variables';

export const onRequest = defineRouteMiddleware(async (context) => {
	const pages = [...(await variablesOverview()), ...(await hclPages()), ...(await commandPages())];
	addDataPages(context.locals.starlightRoute, pages, context.url.pathname);
});

// Returns the link to the variables overview page once that page is published.
async function variablesOverview(): Promise<DataPage[]> {
	if (!(await getEntry('docs', VARIABLES_SLUG))) return [];
	return [{ ...HCL_GROUPS.variables, label: 'Overview', href: `/${VARIABLES_SLUG}/`, experimental: false }];
}

async function hclPages(): Promise<DataPage[]> {
	const kinds = await Promise.all(
		HCL_KINDS.map(async (kind) =>
			Promise.all(
				(await getPublishedHclItems(kind)).map(async (entry) => ({
					...HCL_GROUPS[kind],
					label: entry.data.name,
					href: hclItemPath(kind, entry.data.name),
					experimental: await isExperimental(entry),
				})),
			),
		),
	);
	return kinds.flat();
}

async function commandPages(): Promise<DataPage[]> {
	const commands = (await getPublishedCommands()).sort((a, b) => a.data.sidebar.order - b.data.sidebar.order);
	return commands.map(({ data }) => {
		const [parent, ...rest] = data.path.split('/');
		return {
			group: ['Reference', 'CLI', 'Commands'],
			subgroup: rest.length > 0 ? parent : undefined,
			label: data.name,
			href: `/reference/cli/commands/${data.path}/`,
			experimental: data.experiment !== undefined,
		};
	});
}
