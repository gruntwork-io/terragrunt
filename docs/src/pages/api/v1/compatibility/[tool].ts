import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';

export const prerender = true;

export const getStaticPaths: GetStaticPaths = () => [
	{ params: { tool: 'index' } },
	{ params: { tool: 'opentofu' } },
	{ params: { tool: 'terraform' } },
];

export const GET: APIRoute = async ({ params }) => {
	const tool = params.tool === 'index' ? undefined : params.tool;
	const entries = (await getCollection('compatibility'))
		.filter(e => !tool || e.data.tool === tool)
		.sort((a, b) => {
			if (a.data.tool !== b.data.tool) {
				return a.data.tool === 'opentofu' ? -1 : 1;
			}
			return b.data.order - a.data.order;
		})
		.map(e => ({
			tool: e.data.tool,
			version: e.data.version,
			terragrunt_min: e.data.terragrunt_min,
			terragrunt_max: e.data.terragrunt_max,
		}));

	return new Response(JSON.stringify(entries), {
		headers: { 'Content-Type': 'application/json' },
	});
};
