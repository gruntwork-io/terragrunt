import { getEntry, type CollectionEntry } from 'astro:content';
import { isFlagVisible } from '@lib/flags';

export async function getHeadings(
	command: CollectionEntry<'commands'>,
): Promise<{ depth: number; slug: string; text: string }[]> {
	const headings: { depth: number; slug: string; text: string }[] = [];

	headings.push({ depth: 2, slug: 'usage', text: 'Usage' });

	if (command.data.examples) {
		headings.push({ depth: 2, slug: 'examples', text: 'Examples' });
	}

	// Matched in one pass so the body's headings keep their document order. A
	// pass per depth would list every h3 after the last h2, detaching each from
	// the section it belongs to.
	const bodyHeadingLines = command.body?.match(/^(#{2,3}) (.*)/gm);

	if (bodyHeadingLines) {
		bodyHeadingLines.forEach((line) => {
			const depth = line.startsWith('### ') ? 3 : 2;
			const text = line.replace(/^#{2,3} /, '');
			const slug = text.toLowerCase().replace(/ /g, '-');
			headings.push({ depth, slug, text });
		});
	}

	if (command.data.flags) {
		const flagEntries = await Promise.all(
			command.data.flags.map((flagName: string) => getEntry('flags', flagName)),
		);
		const visibleFlags = (
			await Promise.all(
				flagEntries.map(async (flag) => {
					if (!flag) return null;
					return (await isFlagVisible(flag.data.since)) ? flag : null;
				}),
			)
		).filter((flag): flag is NonNullable<typeof flag> => flag !== null);

		if (visibleFlags.length > 0) {
			headings.push({ depth: 2, slug: 'flags', text: 'Flags' });

			for (const flag of visibleFlags) {
				headings.push({
					depth: 3,
					slug: flag.data.name,
					text: `--${flag.data.name}`,
				});
			}
		}
	}

	return headings;
};
