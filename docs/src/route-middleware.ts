import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { getPublishedCommands } from '@lib/commands/published';
import { HCL_KINDS, hclItemPath, type HclKind } from '@lib/hcl';
import { getPublishedHclItems, isExperimental } from '@lib/hcl/published';
import { addDataPages, type DataPage } from '@lib/sidebar';

const HCL_GROUPS: Record<HclKind, string> = {
	blocks: 'Blocks',
	attributes: 'Attributes',
	functions: 'Functions',
};

export const onRequest = defineRouteMiddleware(async (context) => {
	const pages = [...(await hclPages()), ...(await commandPages())];
	addDataPages(context.locals.starlightRoute, pages, context.url.pathname);
});

async function hclPages(): Promise<DataPage[]> {
	const kinds = await Promise.all(
		HCL_KINDS.map(async (kind) =>
			Promise.all(
				(await getPublishedHclItems(kind)).map(async (entry) => ({
					group: ['Reference', 'HCL', HCL_GROUPS[kind]],
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
