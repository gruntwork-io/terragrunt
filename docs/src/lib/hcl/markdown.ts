import { createMarkdownProcessor } from '@astrojs/markdown-remark';

const renderer = await createMarkdownProcessor({ syntaxHighlight: false });

/** Renders a Markdown field from an HCL entry's frontmatter, e.g. an attribute description, to HTML. */
export async function renderMarkdownField(markdown: string): Promise<string> {
	return (await renderer.render(markdown)).code;
}
