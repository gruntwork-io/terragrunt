import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { HCL_KINDS, hclItemPath } from '@lib/hcl';
import { getPublishedHclItems } from '@lib/hcl/published';
import { sortHclSidebar } from '@lib/hcl/sidebar';

// Starlight sorts the HCL item pages it lists from their placeholders by name.
// This puts them in the order of the index tables.
export const onRequest = defineRouteMiddleware(async (context) => {
	const hrefs = await Promise.all(
		HCL_KINDS.map(async (kind) =>
			(await getPublishedHclItems(kind)).map((entry) => hclItemPath(kind, entry.data.name)),
		),
	);
	sortHclSidebar(context.locals.starlightRoute, hrefs.flat());
});
