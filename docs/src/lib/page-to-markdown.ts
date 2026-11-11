import mdxServer from '@astrojs/mdx/server.js';
import type { APIContext } from 'astro';
import { experimental_AstroContainer } from 'astro/container';
import { render, type CollectionEntry } from 'astro:content';
import { htmlToMarkdown } from '@lib/html-to-markdown';
import type { MarkdownDetail } from '@lib/markdown-document';

/** A content-collection entry that can be rendered to HTML via `render()`. */
type RenderableEntry = CollectionEntry<
	| 'docs'
	| 'commands'
	| 'experiments'
	| 'strictControls'
	| 'changelog'
	| 'faq'
	| 'patterns'
	| 'flags'
	| 'hclBlocks'
	| 'hclAttributes'
	| 'hclFunctions'
>;

type Component = Parameters<typeof container.renderToString>[0];

const container = await experimental_AstroContainer.create({
	renderers: [{ name: 'astro:jsx', ssr: mdxServer }],
});

// Rendered HTML per entry, keyed by collection and id. An entry's `.md` route,
// `llms-full.txt`, and `llms-small.txt` all need the same HTML and differ only
// in the Markdown pass, so the container renders each entry once per build.
// Holding the promise rather than the result lets concurrent routes share one
// render. Entry bodies never read the request, so the first route's context
// serves every later one.
const renderedEntries = new Map<string, Promise<string>>();

/** Renders a content-collection entry, resolving its MDX components, to Markdown. */
export async function entryToMarkdown(
	entry: RenderableEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<string> {
	return htmlToMarkdown(await entryToHtml(entry, context), detail);
}

/**
 * Like {@link entryToMarkdown}, but falls back to a raw-source conversion if the
 * container render throws (e.g. the entry uses the `<Code>` component).
 */
export async function entryToMarkdownOrSource(
	entry: RenderableEntry,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<string> {
	try {
		return await entryToMarkdown(entry, context, detail);
	} catch {
		return rawMdxToMarkdown(entry.body ?? '');
	}
}

/**
 * Renders an Astro component to Markdown. `Component` is an Astro component
 * module's default export. The generated reference pages use this because a
 * component produces their visible content.
 */
export async function componentToMarkdown(
	Component: Component,
	props: Record<string, unknown>,
	context: APIContext,
	detail: MarkdownDetail = 'full',
): Promise<string> {
	return htmlToMarkdown(await componentToHtml(Component, props, context), detail);
}

function entryToHtml(entry: RenderableEntry, context: APIContext): Promise<string> {
	const key = `${entry.collection}/${entry.id}`;
	const cached = renderedEntries.get(key);
	if (cached) return cached;

	const html = render(entry).then(({ Content }) => componentToHtml(Content, {}, context));
	renderedEntries.set(key, html);
	return html;
}

function componentToHtml(
	Component: Component,
	props: Record<string, unknown>,
	context: APIContext,
): Promise<string> {
	return container.renderToString(Component, {
		props,
		request: context.request,
		params: context.params,
		locals: context.locals,
	});
}

/**
 * Best-effort raw-source fallback for entries whose MDX uses components that
 * can't be rendered in the container (notably Expressive Code's `<Code>`
 * component, which needs the EC engine). Drops imports/exports, converts inline
 * `<Code … code={`…`} />` to fenced blocks, and unwraps simple wrapper
 * components, keeping their inner prose. Only the body Markdown is affected.
 */
function rawMdxToMarkdown(body: string): string {
	return body
		.replace(/^[ \t]*import\s[\s\S]*?$/gm, '') // import lines
		.replace(/^[ \t]*export\s[\s\S]*?$/gm, '') // export lines
		// Inline `<Code … lang="x" … code={`…`} … />` → fenced code block.
		.replace(
			/<Code\b[^>]*?\blang=["']([^"']+)["'][^>]*?\bcode=\{`([\s\S]*?)`\}[^>]*?\/>/g,
			(_m, lang, code) => '```' + lang + '\n' + code + '\n```',
		)
		// Remaining `<Code … />` (e.g. code from an imported variable) can't be
		// resolved from source, so drop the tag.
		.replace(/<Code\b[^>]*?\/>/g, '')
		// Unwrap simple wrapper components, keeping inner content.
		.replace(/<\/?(?:Aside|Before|Since|Tabs|TabItem|StatefulTabs|Fragment|Card|Badge)\b[^>]*>/g, '')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
}
