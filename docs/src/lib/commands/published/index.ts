import { getCollection, type CollectionEntry } from 'astro:content';
import { isPublished } from '@lib/release';

/**
 * Returns the commands whose `since` release has shipped, or every command in
 * dev.
 */
export async function getPublishedCommands(): Promise<CollectionEntry<'commands'>[]> {
	const commands = await getCollection('commands');
	const visible = await Promise.all(commands.map((command) => isPublished(command.data.since)));
	return commands.filter((_, i) => visible[i]);
}
