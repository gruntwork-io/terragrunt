// Clean Markdown for the Patterns overview (`/guides/patterns`). The HTML page
// is a grid of cards linking to each pattern's own page; the `.md` mirrors it as
// a list of patterns (title, author, description) linking to each pattern's page.

import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';
import { sortPatternEntries } from '@lib/patterns';
import { markdownDocument } from '@lib/page-to-markdown';

export const prerender = true;

const SUBHEAD =
  'Real-world patterns and examples for building with Terragrunt, ' +
  'contributed by the community.';

export const GET: APIRoute = async () => {
  const patterns = sortPatternEntries(await getCollection('patterns'));

  // The subhead is emitted as the document's `>` description by
  // markdownDocument, so the body is just the list of patterns.
  const sections: string[] = [];

  if (patterns.length === 0) {
    sections.push('_No patterns have been added yet._');
  } else {
    for (const entry of patterns) {
      sections.push(
        `## [${entry.data.title}](/guides/patterns/${entry.id})\n\n` +
          `_By ${entry.data.author}_\n\n` +
          entry.data.description,
      );
    }
  }

  return new Response(
    markdownDocument('Terragrunt Patterns', SUBHEAD, sections.join('\n\n')),
    { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } },
  );
};
