import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { readChangelogEntries } from "./changelog-files";

let changelogDir: string;

function writeEntry(file: string, frontmatter: string, body: string) {
  const target = path.join(changelogDir, file);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, `---\n${frontmatter}\n---\n\n${body}\n`);
}

beforeEach(() => {
  changelogDir = fs.mkdtempSync(path.join(os.tmpdir(), "changelog-files-"));
});

afterEach(() => {
  fs.rmSync(changelogDir, { recursive: true });
});

describe("readChangelogEntries", () => {
  test("reads the frontmatter and body of every entry", () => {
    writeEntry("v1.2.3/fix.mdx", 'version: "v1.2.3"\ncategory: "bug-fixes"\norder: 1', "#### A fix");
    writeEntry("v1.2.2/notes.md", 'version: "v1.2.2"\ncategory: "new-features"', "#### A feature");

    const entries = readChangelogEntries(changelogDir).sort((a, b) => a.id.localeCompare(b.id));

    expect(entries.map((e) => e.data)).toEqual([
      { version: "v1.2.2", category: "new-features" },
      { version: "v1.2.3", category: "bug-fixes", order: 1 },
    ]);
    expect(entries.map((e) => e.body?.trim())).toEqual(["#### A feature", "#### A fix"]);
  });

  test("gives each entry the id of the collection's loader", () => {
    writeEntry("v1.2.3/Fix_Quoting.mdx", 'version: "v1.2.3"\ncategory: "bug-fixes"', "#### A fix");

    expect(readChangelogEntries(changelogDir).map((e) => e.id)).toEqual(["v123/fix_quoting"]);
  });

  test("skips files that are not Markdown", () => {
    fs.writeFileSync(path.join(changelogDir, "README.txt"), "not an entry");

    expect(readChangelogEntries(changelogDir)).toEqual([]);
  });

  test("rejects an entry without frontmatter", () => {
    fs.writeFileSync(path.join(changelogDir, "bare.mdx"), "#### No frontmatter\n");

    expect(() => readChangelogEntries(changelogDir)).toThrow("bare.mdx");
  });

  test("rejects an unknown category", () => {
    writeEntry("v1.2.3/odd.mdx", 'version: "v1.2.3"\ncategory: "misc"', "#### Odd");

    expect(() => readChangelogEntries(changelogDir)).toThrow("category");
  });

  test("rejects a version without the v prefix", () => {
    writeEntry("v1.2.3/odd.mdx", 'version: "1.2.3"\ncategory: "bug-fixes"', "#### Odd");

    expect(() => readChangelogEntries(changelogDir)).toThrow("vX.Y.Z");
  });
});
