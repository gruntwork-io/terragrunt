import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { getLatestRelease } from '@lib/github';
import { effectiveTerragruntMax, filterByReleasedMin } from '@lib/compatibility';

export const prerender = true;

export const getStaticPaths: GetStaticPaths = () => [
	{ params: { tool: 'index' } },
	{ params: { tool: 'opentofu' } },
	{ params: { tool: 'terraform' } },
];

export const GET: APIRoute = async ({ params }) => {
	const tool = params.tool === 'index' ? undefined : params.tool;

	const releaseData = await getLatestRelease('gruntwork-io', 'terragrunt');
	// Fail open when we cannot determine the latest release, so the API still
	// returns historical compatibility data if the GitHub API is unreachable.
	const showUnreleased = import.meta.env.DEV || releaseData === null;
	const latestVersion = (releaseData?.tag_name ?? 'v0.0.0').replace(/^v/, '');

	const filtered = filterByReleasedMin(
		(await getCollection('compatibility')).filter(e => !tool || e.data.tool === tool),
		latestVersion,
		showUnreleased,
	);

	const entries = filtered
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
			terragrunt_max: effectiveTerragruntMax(e.data.terragrunt_max, latestVersion, showUnreleased),
		}));

	return new Response(JSON.stringify(entries), {
		headers: { 'Content-Type': 'application/json' },
	});
};
