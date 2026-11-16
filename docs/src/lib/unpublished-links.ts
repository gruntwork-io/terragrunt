import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";
import picomatch from "picomatch";
import { HCL_KINDS, hclItemSlug, type HclKind } from "./hcl";
import { isPublished } from "./release";

interface LinkContext {
  file: string;
  link: string;
}

/**
 * Builds the `exclude` function for starlight-links-validator. It excludes:
 *
 * - A link matching one of `patterns`.
 * - A link to a published command or HCL page, or to an anchor on one. Their
 *   `.astro` routes render them from data collections, and the validator only
 *   accepts links to pages it has headings for, which it collects from `docs`
 *   entries. A link to a command or HCL item that doesn't exist still fails.
 * - A link from unpublished content to an unpublished page, since neither end
 *   is in the build: e.g. an unreleased changelog entry linking to the page of
 *   the command it announces.
 * - A link to an unpublished page that a file only makes inside
 *   `<Since version="...">` blocks for unreleased versions, since the build
 *   leaves those blocks out. Any other link from published content to an
 *   unpublished page still fails.
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
  const dataDir = path.join(srcDir, "data");
  const changelogDir = path.join(dataDir, "changelog");

  const dataPaths = new Set<string>();
  const unpublishedFiles = new Set<string>();
  const unpublishedPaths = new Set<string>();
  const gatedLinks = new Map<string, Set<string>>();

  for (const file of [...markdownFiles(docsDir), ...markdownFiles(dataDir)]) {
    const { data, content } = matter(fs.readFileSync(file, "utf-8"));
    const since = file.startsWith(changelogDir + path.sep) ? data.version : data.since;
    const published = await isPublished(since);

    if (published) {
      const gated = await unreleasedSinceLinks(content);
      if (gated.size > 0) gatedLinks.set(file, gated);
    }

    if (file.startsWith(docsDir + path.sep)) {
      if (published) continue;
      if (typeof data.slug !== "string") {
        throw new Error(`${file} sets since, so it must also set slug`);
      }
      unpublishedFiles.add(file);
      unpublishedPaths.add(`/${data.slug}`);
      continue;
    }

    const page = dataPagePath(path.relative(dataDir, file), data);
    if (published) {
      if (page) dataPaths.add(page);
      continue;
    }
    unpublishedFiles.add(file);
    if (page) unpublishedPaths.add(page);
  }

  return ({ file, link }) =>
    isExcluded(link.split("?")[0]) ||
    dataPaths.has(linkPath(link)) ||
    (unpublishedPaths.has(linkPath(link)) &&
      (unpublishedFiles.has(file) || gatedLinks.get(file)?.has(link) === true));
}

const SINCE_BLOCK = /<Since\s+version="([^"]+)"\s*>([\s\S]*?)<\/Since>/g;
const LINK_URL = /\]\(\s*<?([^\s)>]+)|href="([^"]+)"/g;

// Returns the links that `content` makes only inside `<Since>` blocks for
// unreleased versions.
async function unreleasedSinceLinks(content: string): Promise<Set<string>> {
  const gated = new Set<string>();
  let rest = content;
  for (const [block, version, body] of content.matchAll(SINCE_BLOCK)) {
    if (await isPublished(version)) continue;
    for (const url of linkUrls(body)) gated.add(url);
    rest = rest.replace(block, "");
  }
  for (const url of linkUrls(rest)) gated.delete(url);
  return gated;
}

function linkUrls(markdown: string): string[] {
  return [...markdown.matchAll(LINK_URL)].map(([, url, href]) => url ?? href);
}

// Returns the path of the page rendered from a data file, given its path
// relative to `src/data/`, or undefined when no page is.
function dataPagePath(file: string, data: Record<string, unknown>): string | undefined {
  const [collection, kind] = file.split(path.sep);
  if (collection === "commands") return `/reference/cli/commands/${data.path}`;
  if (collection === "hcl" && isHclKind(kind)) return `/${hclItemSlug(kind, String(data.name))}`;
  return undefined;
}

function isHclKind(kind: string | undefined): kind is HclKind {
  return HCL_KINDS.some((k) => k === kind);
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
