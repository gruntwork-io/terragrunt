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

	const h2HeadingsLines = command.body?.match(/## (.*)/g);
	const h2Headings = h2HeadingsLines?.map((line) => line.replace(/## /g, ''));

	const h3HeadingsLines = command.body?.match(/### (.*)/g);
	const h3Headings = h3HeadingsLines?.map((line) => line.replace(/### /g, ''));


	if (h2Headings) {
		h2Headings.forEach((text) => {
			const slug = text.toLowerCase().replace(/ /g, '-');
			headings.push({ depth: 2, slug, text });
		});
	}

	if (h3Headings) {
		h3Headings.forEach((text) => {
			const slug = text.toLowerCase().replace(/ /g, '-');
			headings.push({ depth: 3, slug, text });
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
