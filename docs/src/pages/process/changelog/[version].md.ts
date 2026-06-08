// Clean Markdown for the generated per-release changelog pages
// (`/process/changelog/<version>`), rendered by the sibling `[version].astro`
// route from the `changelog` collection. Renders the curated `ReleaseEntries`
// (the human-written release notes) to Markdown.
//
// The HTML page also appends a GitHub-derived "Pull Requests" section; we omit
// it here to keep the `.md` free of build-time GitHub API dependencies — the
// curated entries are the meaningful content for this format.

import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import ReleaseEntries from '@components/ReleaseEntries.astro';
import { entriesForVersion, isReleased, uniqueVersions } from '@lib/changelog';
import { getLatestRelease } from '@lib/github';
import { componentToSimpleMarkdown, markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

export const getStaticPaths = (async () => {
	// Mirror `[version].astro`: include only released versions in a production
	// build (DEV additionally shows unreleased ones).
	const showUnreleased = import.meta.env.DEV;
	const releaseData = await getLatestRelease('gruntwork-io', 'terragrunt');
	const latestVersion = (releaseData?.tag_name ?? 'v0.0.0').replace(/^v/, '');

	const entries = await getCollection('changelog');
	const versions = uniqueVersions(entries).filter(
		(version) => showUnreleased || isReleased(version, latestVersion),
	);

	return versions.map((version) => ({ params: { version }, props: { version } }));
}) satisfies GetStaticPaths;

export const GET: APIRoute = async (context) => {
	const { version } = context.props as { version: string };

	const allEntries = await getCollection('changelog');
	const entries = entriesForVersion(allEntries, version);

	const body = await componentToSimpleMarkdown(ReleaseEntries, { entries }, context);

	return new Response(
		markdownDocument(version, `Notable changes in Terragrunt ${version}.`, body),
		{ headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
	);
};
