import { getCollection, getEntry } from 'astro:content';
import { isCompleted } from '@lib/experiments';
import { getLatestVersion, isPublished } from '@lib/release';
import { compareHclItems, HCL_COLLECTIONS, hclItemSlug, type BlockAttribute, type HclEntry, type HclKind } from '.';

/**
 * Returns the entries of one kind whose `since` release has shipped, or every
 * entry in dev, sorted by name with entries gated by an active experiment last.
 *
 * Each entry has a placeholder page under
 * `src/content/docs/04-reference/01-hcl/<kind>/` so that Starlight's sidebar
 * lists it and the link checker knows its URL. The placeholder's `title` must
 * be the entry's `name`, and its `since` must match the entry's. The docs
 * loader drops an unpublished placeholder; one left behind would publish an
 * empty page at the entry's URL, and one dropped too early would hide a
 * published entry from the sidebar. Any mismatch fails the build.
 */
export async function getPublishedHclItems(kind: HclKind): Promise<HclEntry[]> {
	const entries: HclEntry[] = await getCollection(HCL_COLLECTIONS[kind]);

	const published = await Promise.all(
		entries.map(async (entry) => {
			const { name, since } = entry.data;
			const visible = await isPublished(since);
			const slug = hclItemSlug(kind, name);
			const placeholder = await getEntry('docs', slug);

			if (visible !== (placeholder !== undefined)) {
				throw new Error(
					`${kind} entry "${name}" has since ${since ?? '(none)'}, so its placeholder page ` +
						`with slug ${slug} must ${visible ? 'exist' : 'set the same since'}`,
				);
			}

			if (placeholder && (placeholder.data.since !== since || placeholder.data.title !== name)) {
				throw new Error(
					`placeholder page ${slug} must have title "${name}" and since ${since ?? '(none)'}, ` +
						`but has title "${placeholder.data.title}" and since ${placeholder.data.since ?? '(none)'}`,
				);
			}

			return visible ? withPublishedAttributes(entry) : null;
		}),
	);

	const visible = published.filter((entry): entry is HclEntry => entry !== null);
	const experimental = await Promise.all(visible.map(isExperimental));
	return visible
		.map((entry, i) => ({ entry, name: entry.data.name, experimental: experimental[i] }))
		.sort(compareHclItems)
		.map(({ entry }) => entry);
}

/** Returns true while the experiment named by the entry's `experiment` is active. */
async function isExperimental(entry: HclEntry): Promise<boolean> {
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
