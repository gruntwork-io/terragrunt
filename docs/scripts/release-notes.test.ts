import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { buildReleaseNotes, changelogVersion, type GitHub } from "./release-notes";

const HEAD = "a1b2c3d4e5f6789012345678901234567890abcd";

let changelogDir: string;

function writeEntry(file: string, frontmatter: string, body: string) {
  const target = path.join(changelogDir, file);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, `---\n${frontmatter}\n---\n\n${body}\n`);
}

function github(overrides: Partial<GitHub> = {}): GitHub {
  return {
    latestReleaseTag: async () => "v1.2.2",
    commitsBetween: async () => [
      {
        sha: HEAD,
        html_url: "",
        commit: { message: "fix: Quote generated keys (#7001)" },
        author: { login: "octocat" },
      },
    ],
    ...overrides,
  };
}

beforeEach(() => {
  changelogDir = fs.mkdtempSync(path.join(os.tmpdir(), "release-notes-"));
  writeEntry(
    "v1.2.3/fix-quoting.mdx",
    'version: "v1.2.3"\ncategory: "bug-fixes"',
    "#### Generated keys are quoted\n\nSee the [docs](/reference/hcl/).",
  );
  writeEntry(
    "v1.2.3/new-block.mdx",
    'version: "v1.2.3"\ncategory: "new-features"',
    "#### A new block",
  );
  writeEntry(
    "v1.2.2/older.mdx",
    'version: "v1.2.2"\ncategory: "bug-fixes"',
    "#### An older fix",
  );
});

afterEach(() => {
  fs.rmSync(changelogDir, { recursive: true });
});

describe("changelogVersion", () => {
  test("returns a release version unchanged", () => {
    expect(changelogVersion("v1.2.3")).toBe("v1.2.3");
  });

  test("drops a pre-release suffix", () => {
    expect(changelogVersion("v1.2.3-rc1")).toBe("v1.2.3");
    expect(changelogVersion("v1.2.3-alpha.1")).toBe("v1.2.3");
  });

  test("drops build metadata", () => {
    expect(changelogVersion("v1.2.3+build.5")).toBe("v1.2.3");
  });

  test("rejects a version without the v prefix", () => {
    expect(() => changelogVersion("1.2.3")).toThrow("vX.Y.Z");
  });
});

describe("buildReleaseNotes", () => {
  test("lists changelog entries by category, then pull requests", async () => {
    const notes = await buildReleaseNotes("v1.2.3", HEAD, changelogDir, github());
    expect(notes).toBe(
      [
        "## ✨ New Features",
        "## A new block",
        "## 🐛 Bug Fixes",
        "## Generated keys are quoted",
        "See the [docs](https://docs.terragrunt.com/reference/hcl/).",
        "## Pull Requests",
        "### 🐛 Bug Fixes",
        "- fix: Quote generated keys by [@octocat](https://github.com/octocat) in [#7001](https://github.com/gruntwork-io/terragrunt/pull/7001)",
      ].join("\n\n"),
    );
  });

  test("selects entries by frontmatter version, whatever the directory", async () => {
    writeEntry("misfiled/moved.mdx", 'version: "v1.2.3"\ncategory: "bug-fixes"', "#### Moved");
    const notes = await buildReleaseNotes("v1.2.3", HEAD, changelogDir, github());
    expect(notes).toContain("## Moved");
  });

  test("gives a pre-release the entries of its release version", async () => {
    const notes = await buildReleaseNotes("v1.2.3-rc1", HEAD, changelogDir, github());
    expect(notes).toContain("## A new block");
    expect(notes).not.toContain("An older fix");
  });

  test("compares the latest release against the head commit", async () => {
    const calls: string[][] = [];
    await buildReleaseNotes(
      "v1.2.3",
      HEAD,
      changelogDir,
      github({
        commitsBetween: async (base, head) => {
          calls.push([base, head]);
          return [];
        },
      }),
    );
    expect(calls).toEqual([["v1.2.2", HEAD]]);
  });

  test("fails when the version has no changelog entries", async () => {
    await expect(buildReleaseNotes("v9.9.9", HEAD, changelogDir, github())).rejects.toThrow(
      "no changelog entries for v9.9.9",
    );
  });

  test("fails when the latest release cannot be resolved", async () => {
    const offline = github({ latestReleaseTag: async () => null });
    await expect(buildReleaseNotes("v1.2.3", HEAD, changelogDir, offline)).rejects.toThrow(
      "latest release",
    );
  });

  test("fails when the commits cannot be listed", async () => {
    const offline = github({ commitsBetween: async () => null });
    await expect(buildReleaseNotes("v1.2.3", HEAD, changelogDir, offline)).rejects.toThrow(
      "commits between v1.2.2",
    );
  });
});
