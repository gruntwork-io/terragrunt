import type { Element, ElementContent, Root } from 'hast';
import { matches, select, selectAll } from 'hast-util-select';
import rehypeParse from 'rehype-parse';
import rehypeRemark, { type Options as RehypeRemarkOptions } from 'rehype-remark';
import remarkGfm, { type Options as RemarkGfmOptions } from 'remark-gfm';
import remarkStringify, { type Options as RemarkStringifyOptions } from 'remark-stringify';
import { unified } from 'unified';
import type { MarkdownDetail } from '@lib/markdown-document';

const SITE_CHROME = 'a.sl-anchor-link, a.anchor-link, .sr-only';

const LOW_VALUE_CONTENT = 'details, .starlight-aside--note, .starlight-aside--tip';

// Blocks that Markdown parses the same way with or without a blank line next to
// them. Every other pair keeps its blank line: a paragraph straight after a
// list, blockquote, or table would be read as part of it, and two paragraphs
// would merge.
const SELF_DELIMITING_BLOCKS = new Set(['heading', 'code']);

const handlers: NonNullable<RehypeRemarkOptions['handlers']> = {
	pre: (_state, pre: Element) => {
		const lang = pre.properties.dataLanguage as string | undefined;
		const title = pre.properties.dataTitle as string | undefined;
		const meta = title ? `title=${JSON.stringify(title)}` : undefined;
		const lines = selectAll('.ec-line', pre);
		if (lines.length === 0) {
			return { type: 'code', lang, meta, value: textOf(pre).replace(/\n$/, '') };
		}
		const markers = lang === 'diff' ? [] : lines.map(diffMarker);
		const isDiff = markers.some((marker) => marker !== '');
		return {
			type: 'code',
			lang: isDiff ? 'diff' : lang,
			meta,
			value: lines
				.map((line, index) => (markers[index] ?? '') + textOf(line).replace(/\n$/, ''))
				.join('\n'),
		};
	},
	'starlight-tabs': (state, tabs: Element) => {
		const labels = selectAll('[role="tab"]', tabs);
		const panels = selectAll('[role="tabpanel"]', tabs);
		return {
			type: 'list',
			ordered: false,
			spread: true,
			children: panels.map((panel, index) => {
				const label = labels[index];
				return {
					type: 'listItem',
					spread: true,
					children: [
						...(label
							? [{ type: 'paragraph' as const, children: [{ type: 'text' as const, value: textOf(label).trim() }] }]
							: []),
						...state.toFlow(state.all(panel)),
					],
				};
			}),
		};
	},
};

const nodeHandlers: NonNullable<RehypeRemarkOptions['nodeHandlers']> = {
	comment: () => undefined,
};

const processors = {
	full: createProcessor(SITE_CHROME),
	small: createProcessor(
		`${SITE_CHROME}, ${LOW_VALUE_CONTENT}`,
		{ tablePipeAlign: false },
		{
			join: [
				(left, right) =>
					SELF_DELIMITING_BLOCKS.has(left.type) || SELF_DELIMITING_BLOCKS.has(right.type)
						? 0
						: undefined,
			],
		},
	),
};

/** Converts rendered page HTML to Markdown. */
export async function htmlToMarkdown(html: string, detail: MarkdownDetail = 'full'): Promise<string> {
	const file = await processors[detail].process(html);
	return String(file).trim();
}

function createProcessor(
	ignoredElements: string,
	gfmOptions: RemarkGfmOptions = {},
	stringifyOptions: RemarkStringifyOptions = {},
) {
	return unified()
		.use(rehypeParse, { fragment: true })
		.use(() => (tree: Root) => {
			moveCodeTitles(tree);
			for (const element of selectAll(ignoredElements, tree)) {
				// `rehype-remark` skips elements that carry this marker.
				element.properties.dataMdast = 'ignore';
			}
		})
		.use(rehypeRemark, { handlers, nodeHandlers })
		.use(remarkGfm, gfmOptions)
		.use(remarkStringify, stringifyOptions);
}

/**
 * Moves the title that Expressive Code shows above a code block, e.g. a file
 * name, from the block's caption to its `pre`, so that the `pre` handler writes
 * it into the fence as `title="…"` rather than a paragraph above the code.
 */
function moveCodeTitles(tree: Root): void {
	for (const figure of selectAll('figure', tree)) {
		const pre = select('pre', figure);
		const caption = select('figcaption', figure);
		if (!pre || !caption) continue;
		const title = select('.title', caption);
		if (title) pre.properties.dataTitle = textOf(title).trim();
		caption.properties.dataMdast = 'ignore';
	}
}

/** The diff prefix for an Expressive Code line marked as inserted or deleted. */
function diffMarker(line: Element): '+' | '-' | '' {
	if (matches('.ins', line)) return '+';
	if (matches('.del', line)) return '-';
	return '';
}

function textOf(node: ElementContent): string {
	if (node.type === 'text') return node.value;
	return node.type === 'element' ? node.children.map(textOf).join('') : '';
}
