// Renders an Astro content-collection entry to clean Markdown by rendering it
// to HTML (resolving MDX components) and converting the HTML back to Markdown.
//
// This is a vendored adaptation of `starlight-llms-txt`'s `entryToSimpleMarkdown`
// (MIT licensed, v0.9.0, https://github.com/HiDeoo/starlight-llms-txt). The plugin
// only exposes this conversion through its `/llms*.txt` routes, so to serve the
// same clean Markdown per-page at `<url>.md` we reuse its pipeline here. The only
// substantive change is dropping the `virtual:starlight-llms-txt/context` module
// (which is internal to the plugin) and inlining its configuration, so this module
// is self-contained and usable from our own routes.
//
// Keep this roughly in sync with the plugin when it is upgraded.

import mdxServer from '@astrojs/mdx/server.js';
import type { APIContext } from 'astro';
import { experimental_AstroContainer } from 'astro/container';
import { render, type CollectionEntry } from 'astro:content';
import type { RootContent } from 'hast';
import { matches, select, selectAll } from 'hast-util-select';
import rehypeParse from 'rehype-parse';
import rehypeRemark from 'rehype-remark';
import remarkGfm from 'remark-gfm';
import remarkStringify from 'remark-stringify';
import { unified } from 'unified';
import { remove } from 'unist-util-remove';

// Mirror the plugin's vfile augmentation so the `minify` flag we thread through
// the pipeline below is typed.
declare module 'vfile' {
	interface DataMap {
		starlightLlmsTxt: {
			minify: boolean;
		};
	}
}

/**
 * Minification options. These only take effect when `entryToSimpleMarkdown` is
 * called with `shouldMinify: true`; for full per-page Markdown we render with
 * minification off so asides, details, etc. are preserved. Mirrors the plugin
 * defaults so behaviour matches the existing `/llms-small.txt` output.
 */
const minify = {
	note: true,
	tip: true,
	caution: false,
	danger: false,
	details: true,
	whitespace: true,
	collapseCodeBlocks: false,
	customSelectors: [] as string[],
};

/** Selectors for elements to remove during minification. */
const selectors = [...minify.customSelectors];
if (minify.details) selectors.unshift('details');

const astroContainer = await experimental_AstroContainer.create({
	renderers: [{ name: 'astro:jsx', ssr: mdxServer }],
});

const htmlToMarkdownPipeline = unified()
	.use(rehypeParse, { fragment: true })
	.use(function minifyLlmsTxt() {
		return (tree, file) => {
			if (!file.data.starlightLlmsTxt?.minify) {
				return;
			}
			remove(tree, (_node) => {
				const node = _node as RootContent;

				// Remove elements matching any selectors to be minified:
				for (const selector of selectors) {
					if (matches(selector, node)) {
						return true;
					}
				}

				// Remove aside components:
				if (matches('.starlight-aside', node)) {
					for (const variant of ['note', 'tip', 'caution', 'danger'] as const) {
						if (minify[variant] && matches(`.starlight-aside--${variant}`, node)) {
							return true;
						}
					}
				}

				return false;
			});
			return tree;
		};
	})
	.use(function improveExpressiveCodeHandling() {
		return (tree) => {
			const ecInstances = selectAll('.expressive-code', tree as Parameters<typeof selectAll>[1]);
			for (const instance of ecInstances) {
				// Remove the “Terminal Window” label from Expressive Code terminal frames.
				const figcaption = select('figcaption', instance);
				if (figcaption) {
					const terminalWindowTextIndex = figcaption.children.findIndex((child) =>
						matches('span.sr-only', child),
					);
					if (terminalWindowTextIndex > -1) {
						figcaption.children.splice(terminalWindowTextIndex, 1);
					}
				}
				const pre = select('pre', instance);
				const code = select('code', instance);
				// Use Expressive Code’s `data-language=*` attribute to set a `language-*` class name.
				// This is what `hast-util-to-mdast` checks for code language metadata.
				if (pre?.properties.dataLanguage && code) {
					if (!Array.isArray(code.properties.className)) code.properties.className = [];

					const diffLines =
						pre.properties.dataLanguage === 'diff'
							? []
							: code.children.filter((child) => matches('div.ec-line.ins, div.ec-line.del', child));
					if (diffLines.length === 0) {
						code.properties.className.push(`language-${pre.properties.dataLanguage}`);
					} else {
						code.properties.className.push('language-diff');
						for (const line of diffLines) {
							if (line.type !== 'element') continue;
							const classes = line.properties?.className;
							if (typeof classes !== 'string' && !Array.isArray(classes)) continue;
							const marker = classes.includes('ins') ? '+' : '-';
							const span = select('span:not(.indent)', line);
							const firstChild = span?.children[0];
							if (firstChild?.type === 'text') {
								firstChild.value = `${marker}${firstChild.value}`;
							}
						}
					}
				}
			}
		};
	})
	.use(function improveTabsHandling() {
		return (tree) => {
			const tabInstances = selectAll('starlight-tabs', tree as Parameters<typeof selectAll>[1]);
			for (const instance of tabInstances) {
				const tabs = selectAll('[role="tab"]', instance);
				const panels = selectAll('[role="tabpanel"]', instance);
				// Convert parent `<starlight-tabs>` element to empty unordered list.
				instance.tagName = 'ul';
				instance.properties = {};
				instance.children = [];
				// Iterate over tabs and panels to build a list with tab label as initial list text.
				for (let i = 0; i < Math.min(tabs.length, panels.length); i++) {
					const tab = tabs[i];
					const panel = panels[i];
					if (!tab || !panel) continue;
					// Filter out extra whitespace and icons from tab contents.
					const tabLabel = tab.children
						.filter((child) => child.type === 'text' && child.value.trim())
						.map((child) => child.type === 'text' && child.value.trim())
						.join('');
					// Add list entry for this tab and panel.
					instance.children.push({
						type: 'element',
						tagName: 'li',
						properties: {},
						children: [
							{
								type: 'element',
								tagName: 'p',
								children: [{ type: 'text', value: tabLabel }],
								properties: {},
							},
							panel,
						],
					});
				}
			}
		};
	})
	.use(function improveFileTreeHandling() {
		return (tree) => {
			const trees = selectAll('starlight-file-tree', tree as Parameters<typeof selectAll>[1]);
			for (const tree of trees) {
				// Remove “Directory” screen reader labels from <FileTree> entries.
				remove(tree, (_node) => {
					const node = _node as RootContent;
					return matches('.sr-only', node);
				});
			}
		};
	})
	.use(function removeHtmlComments() {
		return (tree) => {
			remove(tree, ({ type }) => type === 'comment');
		};
	})
	// Addition (not in the upstream plugin): Starlight (and some of this repo's
	// own components) render an empty permalink anchor next to headings —
	// `a.sl-anchor-link` with a screen-reader "Section titled …" label, and
	// `a.anchor-link` in the experiments/strict-controls components. Left in,
	// each heading produces a noisy `[Section titled "X"](#x)` line (or an empty
	// `[](#x)` link) in the output. Strip them so the Markdown is clean.
	.use(function removeHeadingAnchorLinks() {
		return (tree) => {
			remove(tree, (node) =>
				matches('a.sl-anchor-link, a.anchor-link', node as RootContent),
			);
		};
	})
	.use(rehypeRemark)
	.use(remarkGfm)
	.use(remarkStringify);

/** A content-collection entry that can be rendered to HTML via `render()`. */
type RenderableEntry = CollectionEntry<
	'docs' | 'commands' | 'experiments' | 'strictControls' | 'changelog' | 'patterns' | 'flags'
>;

/**
 * Frame converted Markdown as a standalone document: an H1 title, an optional
 * blockquote description, then the body. Matches the framing `starlight-llms-txt`
 * uses for each page in `llms-full.txt`, so the per-page `.md` is consistent.
 */
export function markdownDocument(
	title: string,
	description: string | undefined,
	body: string,
): string {
	const segments = [`# ${title}`];
	if (description) segments.push(`> ${description}`);
	if (body.trim()) segments.push(body);
	return segments.join('\n\n') + '\n';
}

/** Run rendered HTML through the rehype→remark pipeline and tidy whitespace. */
async function htmlToMarkdown(html: string, shouldMinify: boolean) {
	const file = await htmlToMarkdownPipeline.process({
		value: html,
		data: { starlightLlmsTxt: { minify: shouldMinify } },
	});
	let markdown = String(file).trim();
	if (shouldMinify && minify.whitespace) {
		if (minify.collapseCodeBlocks) {
			markdown = markdown.replace(/\s+/g, ' ');
		} else {
			// Collapse whitespace in prose, but keep the contents of fenced code
			// blocks (``` and ~~~, of any length ≥ 3) intact so multi-line code
			// samples stay multi-line.
			const fenceMatcher =
				/(?<=^|\n)([ \t]*)(`{3,}|~{3,})[^\n]*\n(?:[\s\S]*?\n)?\1\2[ \t]*(?=\n|$)/g;
			const parts: string[] = [];
			let lastIndex = 0;
			for (const match of markdown.matchAll(fenceMatcher)) {
				const index = match.index ?? 0;
				parts.push(markdown.slice(lastIndex, index).replace(/\s+/g, ' '));
				parts.push('\n', match[0], '\n');
				lastIndex = index + match[0].length;
			}
			parts.push(markdown.slice(lastIndex).replace(/\s+/g, ' '));
			markdown = parts.join('').trim();
		}
	}
	return markdown;
}

/**
 * Render a content collection entry to HTML and back to Markdown to support
 * rendering and simplifying MDX components.
 */
export async function entryToSimpleMarkdown(
	entry: RenderableEntry,
	context: APIContext,
	shouldMinify: boolean = false,
) {
	const { Content } = await render(entry);
	const html = await astroContainer.renderToString(Content, context);
	return htmlToMarkdown(html, shouldMinify);
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
		// resolved from source — drop the tag.
		.replace(/<Code\b[^>]*?\/>/g, '')
		// Unwrap simple wrapper components, keeping inner content.
		.replace(/<\/?(?:Aside|Before|Since|Tabs|TabItem|Card|Badge)\b[^>]*>/g, '')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
}

/**
 * Like {@link entryToSimpleMarkdown}, but falls back to a raw-source conversion
 * if the container render throws (e.g. the entry uses the `<Code>` component).
 * Keeps the build resilient while still producing clean output for the common
 * case.
 */
export async function safeEntryToMarkdown(
	entry: RenderableEntry,
	context: APIContext,
	shouldMinify: boolean = false,
) {
	try {
		return await entryToSimpleMarkdown(entry, context, shouldMinify);
	} catch {
		return rawMdxToMarkdown(entry.body ?? '');
	}
}

/**
 * Render an arbitrary Astro component (with props) to clean Markdown using the
 * same pipeline. Used for the dynamically-generated reference pages (CLI
 * commands, changelog releases, experiments, strict-controls), whose visible
 * content is produced by a component rather than a markdown body. `Component` is
 * an Astro component module's default export.
 */
export async function componentToSimpleMarkdown(
	Component: Parameters<typeof astroContainer.renderToString>[0],
	props: Record<string, unknown>,
	context: APIContext,
	shouldMinify: boolean = false,
) {
	const html = await astroContainer.renderToString(Component, {
		props,
		request: context.request,
		params: context.params,
	});
	return htmlToMarkdown(html, shouldMinify);
}
