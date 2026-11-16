import { getCollection, getEntry } from 'astro:content';
import { isCompleted } from '@lib/experiments';
import { getLatestVersion, isPublished } from '@lib/release';
import { compareHclItems, HCL_COLLECTIONS, type BlockAttribute, type HclEntry, type HclKind } from '.';

/**
 * Returns the entries of one kind whose `since` release has shipped, or every
 * entry in dev, sorted by name with entries gated by an active experiment last.
 */
export async function getPublishedHclItems(kind: HclKind): Promise<HclEntry[]> {
	const entries: HclEntry[] = await getCollection(HCL_COLLECTIONS[kind]);
	const visible = await Promise.all(entries.map((entry) => isPublished(entry.data.since)));
	const published = await Promise.all(entries.filter((_, i) => visible[i]).map(withPublishedAttributes));

	const experimental = await Promise.all(published.map(isExperimental));
	return published
		.map((entry, i) => ({ entry, name: entry.data.name, experimental: experimental[i] }))
		.sort(compareHclItems)
		.map(({ entry }) => entry);
}

/** Returns true while the experiment named by the entry's `experiment` is active. */
export async function isExperimental(entry: HclEntry): Promise<boolean> {
	const { name, experiment } = entry.data;
	if (experiment === undefined) return false;

	const found = await getEntry('experiments', experiment);
	if (!found) throw new Error(`entry "${name}" names experiment "${experiment}", which doesn't exist`);
	return !isCompleted(found, await getLatestVersion());
}

/**
 * Returns a block entry without the attributes, its own or its nested blocks',
 * and the map keys whose `since` release hasn't shipped.
 */
async function withPublishedAttributes(entry: HclEntry): Promise<HclEntry> {
	if (entry.collection !== 'hclBlocks') return entry;

	const { attributes, blocks } = entry.data;
	return {
		...entry,
		data: {
			...entry.data,
			attributes:
				attributes &&
				(await Promise.all((await publishedOnly(attributes)).map(withPublishedKeys))),
			blocks:
				blocks &&
				(await Promise.all(
					blocks.map(async (block) => ({ ...block, attributes: await publishedOnly(block.attributes) })),
				)),
		},
	};
}

async function withPublishedKeys(attribute: BlockAttribute): Promise<BlockAttribute> {
	const { keyGroups } = attribute;
	if (!keyGroups) return attribute;

	const groups = await Promise.all(
		keyGroups.groups.map(async (group) => ({
			...group,
			keys: await Promise.all(
				(await publishedOnly(group.keys)).map(async (key) => ({
					...key,
					keys: key.keys && (await publishedOnly(key.keys)),
				})),
			),
		})),
	);
	return { ...attribute, keyGroups: { ...keyGroups, groups } };
}

async function publishedOnly<T extends { since?: string }>(items: T[]): Promise<T[]> {
	const visible = await Promise.all(items.map((item) => isPublished(item.since)));
	return items.filter((_, i) => visible[i]);
}
