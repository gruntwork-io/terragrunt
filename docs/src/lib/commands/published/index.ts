import { getCollection, getEntry, type CollectionEntry } from 'astro:content';
import { isPublished } from '@lib/release';

/**
 * Returns the commands whose `since` release has shipped, or every command in
 * dev.
 *
 * Each command has a placeholder page under
 * `src/content/docs/04-reference/02-cli/02-commands/` so that Starlight's
 * sidebar lists it, and the placeholder must carry the command's `since`. The
 * docs loader drops an unpublished placeholder; one left behind would publish
 * an empty page at the command's URL, and one dropped too early would hide a
 * published command from the sidebar. Either mismatch fails the build.
 */
export async function getPublishedCommands(): Promise<CollectionEntry<'commands'>[]> {
	const commands = await getCollection('commands');

	const published = await Promise.all(
		commands.map(async (command) => {
			const { path, since } = command.data;
			const visible = await isPublished(since);
			const placeholder = await getEntry('docs', `reference/cli/commands/${path}`);

			if (visible !== (placeholder !== undefined) || (placeholder && placeholder.data.since !== since)) {
				throw new Error(
					`command "${path}" has since ${since ?? '(none)'}, but its placeholder page ` +
						`under 02-commands/ has since ${placeholder?.data.since ?? '(none)'}; set both to the same release`,
				);
			}

			return visible ? command : null;
		}),
	);

	return published.filter((command): command is CollectionEntry<'commands'> => command !== null);
}
