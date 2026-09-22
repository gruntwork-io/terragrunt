import type { Loader } from "astro/loaders";
import { getLatestRelease } from "./github";
import { isReleased } from "./versions";

/**
 * Returns the tag of the latest Terragrunt release, e.g. `v1.1.6`. When GitHub
 * can't be reached it returns `v0.0.0`, which treats every gated version as
 * unreleased.
 */
export async function getLatestVersion(): Promise<string> {
  const release = await getLatestRelease("gruntwork-io", "terragrunt");
  return release?.tag_name ?? "v0.0.0";
}

/**
 * Returns true when content gated on `since` belongs in this build: always in
 * dev, and otherwise once the latest release reaches `since`. Content with no
 * `since` is always published.
 */
export async function isPublished(since: string | undefined): Promise<boolean> {
  if (import.meta.env.DEV) return true;
  if (!since) return true;
  return isReleased(since, await getLatestVersion());
}

/**
 * Wraps a content loader so that it drops every entry whose `since`
 * frontmatter is unpublished. A dropped docs page gets no route, no sidebar
 * entry, and no search result, and a link to it fails the link check.
 */
export function publishedOnly(loader: Loader): Loader {
  return {
    ...loader,
    async load(context) {
      await loader.load(context);
      for (const [id, entry] of context.store.entries()) {
        const since = entry.data.since as string | undefined;
        if (!(await isPublished(since))) context.store.delete(id);
      }
    },
  };
}
