// Clean Markdown for the FAQ overview (`/process/faq`). The HTML page is a
// "Question | Answer" table linking to each question's own page; the `.md`
// mirrors it as a Markdown table whose questions link to each question's `.md`.

import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';
import { sortFaqEntries } from '@lib/faq';
import { markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

const SUBHEAD =
  "We know there's a lot to learn about Terragrunt. Here are the " +
  'frequently asked questions that users have asked and that you might find ' +
  'helpful!';

// Escape characters that would break a Markdown table cell.
function cell(text: string): string {
  return text.replace(/\|/g, '\\|').replace(/\r?\n/g, ' ').trim();
}

export const GET: APIRoute = async () => {
  const questions = sortFaqEntries(await getCollection('faq'));

  // The subhead is emitted as the document's `>` description by
  // markdownDocument, so the body is just the table.
  const sections: string[] = [];

  if (questions.length === 0) {
    sections.push('_No frequently asked questions have been added yet._');
  } else {
    const rows = questions.map((entry) => {
      const question = `[${cell(entry.data.question)}](/process/faq/${entry.id})`;
      return `| ${question} | ${cell(entry.data.description)} |`;
    });
    sections.push(['| Question | Answer |', '| --- | --- |', ...rows].join('\n'));
  }

  return new Response(
    markdownDocument(
      'Terragrunt Frequently Asked Questions',
      SUBHEAD,
      sections.join('\n\n'),
    ),
    { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
  );
};
