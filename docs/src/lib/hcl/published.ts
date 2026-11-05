import { getCollection, getEntry } from 'astro:content';
import { isPublished } from '@lib/release';
import { HCL_COLLECTIONS, hclItemSlug, type HclEntry, type HclKind } from '.';

/**
 * Returns the entries of one kind whose `since` release has shipped, or every
 * entry in dev, sorted by name.
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

			return visible ? entry : null;
		}),
	);

	return published
		.filter((entry): entry is HclEntry => entry !== null)
		.sort((a, b) => a.data.name.localeCompare(b.data.name));
}
