import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";
import picomatch from "picomatch";
import { isPublished } from "./release";

interface LinkContext {
  file: string;
  link: string;
}

/**
 * Builds the `exclude` function for starlight-links-validator. It excludes a
 * link matching one of `patterns`, and a link from unpublished content to an
 * unpublished page, since neither end is in the build: e.g. an unreleased
 * changelog entry linking to the page of the command it announces. A link from
 * published content to an unpublished page still fails validation.
 *
 * `srcDir` is the absolute path of the site's `src/` directory. A docs page
 * gated on `since` must set `slug`, which is how links name it.
 */
export async function excludeLinks(
  srcDir: string,
  patterns: string[],
): Promise<(context: LinkContext) => boolean> {
  const isExcluded = picomatch(patterns);
  const docsDir = path.join(srcDir, "content", "docs");
  const changelogDir = path.join(srcDir, "data", "changelog");

  const unpublishedFiles = new Set<string>();
  const unpublishedPaths = new Set<string>();

  for (const file of [...markdownFiles(docsDir), ...markdownFiles(path.join(srcDir, "data"))]) {
    const { data } = matter(fs.readFileSync(file, "utf-8"));
    const since = file.startsWith(changelogDir + path.sep) ? data.version : data.since;
    if (await isPublished(since)) continue;

    unpublishedFiles.add(file);
    if (!file.startsWith(docsDir + path.sep)) continue;
    if (typeof data.slug !== "string") {
      throw new Error(`${file} sets since, so it must also set slug`);
    }
    unpublishedPaths.add(`/${data.slug}`);
  }

  return ({ file, link }) =>
    isExcluded(link.split("?")[0]) ||
    (unpublishedFiles.has(file) && unpublishedPaths.has(linkPath(link)));
}

function markdownFiles(dir: string): string[] {
  return fs
    .readdirSync(dir, { recursive: true, encoding: "utf-8" })
    .filter((file) => file.endsWith(".md") || file.endsWith(".mdx"))
    .map((file) => path.join(dir, file));
}

// Returns the page a link names, without its hash, query, or trailing slash.
function linkPath(link: string): string {
  return link.split(/[#?]/)[0].replace(/\/$/, "");
}
