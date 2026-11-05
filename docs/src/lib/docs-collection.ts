/**
 * Reports whether a `docs` entry has content of its own. Placeholders contain
 * only comments, because a dedicated `.astro` route renders their visible
 * content: the `reference/cli/commands/*` pages from the `commands` collection,
 * and the `process/changelog`, `process/faq`, and `guides/patterns` overviews.
 */
export function hasRenderableBody(body: string | undefined): boolean {
	if (!body) return false;
	const stripped = body
		.replace(/<!--[\s\S]*?-->/g, '') // HTML comments (used by CLI command stubs)
		.replace(/\{\/\*[\s\S]*?\*\/\}/g, '') // MDX/JSX comments (used by the changelog stub)
		.trim();
	return stripped.length > 0;
}
