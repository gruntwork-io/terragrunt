import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

type Entry = { tool: string; version: string; terragrunt_min: string; terragrunt_max: string | null };

function filterByTool(entries: Entry[], tool: string | null): Entry[] {
	if (tool === null) return entries;
	if (tool === 'opentofu') return entries.filter(e => e.tool === 'opentofu');
	if (tool === 'terraform') return entries.filter(e => e.tool === 'terraform');
	return [];
}

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
	const filtered = filterByTool(entries, tool);

	return new Response(JSON.stringify(filtered), {
		headers: { 'Content-Type': 'application/json' },
	});
};
