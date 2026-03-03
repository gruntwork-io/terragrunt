import { getEntry, type CollectionEntry } from 'astro:content';

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
		headings.push({ depth: 2, slug: 'flags', text: 'Flags' });

		const flags = await Promise.all(command.data.flags.map(async (flagName: string) => {
			const flag = (await getEntry('flags', flagName))!;
			return {
				depth: 3,
				slug: flag.data.name,
				text: `--${flag.data.name}`,
			};
		}));

		headings.push(...flags);
	}

	return headings;
};
