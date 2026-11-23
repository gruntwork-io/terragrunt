import fs from "node:fs";
import path from "node:path";
import { slug } from "github-slugger";
import matter from "gray-matter";
import { z } from "astro/zod";
import { changelogSchema, type ChangelogEntry } from "./changelog";

const CHANGELOG_FILE = /\.mdx?$/;
const CHANGELOG_README = "README.md";

/**
 * Reads every changelog entry under `dir`, for code that runs outside an Astro
 * build. As in the `changelog` collection, the README.md at the top of `dir`
 * is not an entry, and entries pass the same schema and get the same ids, so
 * they sort the same way. Throws on the first entry the schema rejects.
 */
export function readChangelogEntries(dir: string): ChangelogEntry[] {
  return fs
    .readdirSync(dir, { recursive: true, encoding: "utf-8" })
    .filter((file) => CHANGELOG_FILE.test(file) && file !== CHANGELOG_README)
    .map((file) => {
      const { data, content } = matter(fs.readFileSync(path.join(dir, file), "utf-8"));
      const result = changelogSchema.safeParse(data);
      if (!result.success) {
        throw new Error(`${file}: ${z.prettifyError(result.error)}`);
      }

      return { id: entryId(file), body: content, data: result.data };
    });
}

// Returns the id the collection's glob loader gives `file`: its path without
// the extension, each segment slugged.
function entryId(file: string): string {
  return file.replace(CHANGELOG_FILE, "").split(path.sep)
    .map((segment) => slug(segment))
    .join("/");
}
