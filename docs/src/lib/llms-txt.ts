import type { APIContext } from 'astro';
import { LLMS_PAGE_SOURCES } from '@lib/generated-pages';
import { llmsDocument, type MarkdownDetail, type MarkdownPage } from '@lib/markdown-document';

/** Builds the `llms-full.txt` or `llms-small.txt` response. */
export async function llmsTxtResponse(context: APIContext, detail: MarkdownDetail): Promise<Response> {
	const pages: MarkdownPage[] = [];
	for (const source of LLMS_PAGE_SOURCES) {
		pages.push(...(await source(context, detail)));
	}
	return new Response(llmsDocument(detail, pages));
}
