import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

export const GET: APIRoute = async ({ request }) => {
	const entries = (await getCollection('compatibility'))
		.sort((a, b) => {
			// Sort by tool first (opentofu before terraform), then by order
			if (a.data.tool !== b.data.tool) {
				return a.data.tool === 'opentofu' ? -1 : 1;
			}
			return a.data.order - b.data.order;
		})
		.map(e => ({
			tool: e.data.tool,
			version: e.data.version,
			terragrunt_min: e.data.terragrunt_min,
			terragrunt_max: e.data.terragrunt_max,
		}));

	const url = new URL(request.url);
	const tool = url.searchParams.get('tool');

	// Filter by tool if query param provided
	let filtered = entries;
	if (tool === 'opentofu') {
		filtered = entries.filter(e => e.tool === 'opentofu');
	} else if (tool === 'terraform') {
		filtered = entries.filter(e => e.tool === 'terraform');
	}

	return new Response(JSON.stringify(filtered), {
		headers: { 'Content-Type': 'application/json' },
	});
};
