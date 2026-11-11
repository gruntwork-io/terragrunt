import { SITE_DESCRIPTION, SITE_TITLE } from '../data/site';

/** A page's Markdown before it is framed as a document. */
export interface MarkdownPage {
	title: string;
	description?: string;
	body: string;
}

/**
 * How much of a page to keep: all of it, or the `llms-small.txt` version without
 * notes, tips, collapsible details, or the blank lines Markdown can do without.
 */
export type MarkdownDetail = 'full' | 'small';

const LLMS_INTRO: Record<MarkdownDetail, string> = {
	full: 'This is the full documentation.',
	small: 'This is the abridged documentation. It omits notes, tips, and collapsible details.',
};

/**
 * Frame converted Markdown as a standalone document: an H1 title, an optional
 * blockquote description, then the body.
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

/**
 * Builds the `llms-full.txt` or `llms-small.txt` content: the site's title and
 * description, then a section per page. A page with an empty body gets no
 * section, such as a status page with no entries.
 */
export function llmsDocument(detail: MarkdownDetail, pages: MarkdownPage[]): string {
	const sections = pages
		.filter((page) => page.body.trim())
		.map((page) => markdownDocument(page.title, page.description, page.body));
	return [markdownDocument(SITE_TITLE, SITE_DESCRIPTION, LLMS_INTRO[detail]), ...sections].join('\n');
}
