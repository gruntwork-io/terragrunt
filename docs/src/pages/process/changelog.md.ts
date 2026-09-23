// Clean Markdown for the changelog overview (`/process/changelog`). The HTML
// page is a navigation index (a card per release linking to per-version pages);
// the `.md` mirrors it as a list of releases with their category breakdown and
// links to each release's own `.md`.

import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';
import { entriesForVersion, groupByCategory, uniqueVersions } from '@lib/changelog';
import { getLatestVersion } from '@lib/release';
import { markdownDocument } from '@lib/page-to-markdown';
import { compareVersionsDesc, isReleased } from '@lib/versions';

export const prerender = true;

const RELEASES_URL = 'https://github.com/gruntwork-io/terragrunt/releases';

export const GET: APIRoute = async () => {
	const showUnreleased = import.meta.env.DEV;
	const latestVersion = await getLatestVersion();

	const allEntries = await getCollection('changelog');
	const versions = uniqueVersions(allEntries)
		.filter((version) => showUnreleased || isReleased(version, latestVersion))
		.sort(compareVersionsDesc);

	const sections: string[] = [
		`This page tracks notable changes to Terragrunt. For the full list of ` +
			`releases and their assets, see the [GitHub Releases Page](${RELEASES_URL}).`,
	];

	for (const version of versions) {
		const groups = groupByCategory(entriesForVersion(allEntries, version));
		sections.push(`## [${version}](/process/changelog/${version})`);
		if (groups.length) {
			sections.push(
				groups
					.map((group) => `- ${group.category.label}: ${group.entries.length}`)
					.join('\n'),
			);
		}
	}

	sections.push(
		'## Prior Releases',
		`For all prior releases, see the [GitHub Releases Page](${RELEASES_URL}).`,
	);

	return new Response(
		markdownDocument(
			'Changelog',
			'A log of notable changes to Terragrunt.',
			sections.join('\n\n'),
		),
		{ headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
	);
};
