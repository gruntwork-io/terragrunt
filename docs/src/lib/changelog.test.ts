import { describe, expect, test } from "bun:test";
import {
  categorySlugSort,
  compareVersionsDesc,
  isReleased,
  parsePullRequests,
  prepareForGitHub,
  pullRequestsToMarkdown,
} from "./changelog";

const SITE = "https://terragrunt.gruntwork.io";

describe("compareVersionsDesc", () => {
  test("orders semver versions descending", () => {
    const sorted = ["v1.0.10", "v1.0.2", "v1.0.0"].sort(compareVersionsDesc);
    expect(sorted).toEqual(["v1.0.10", "v1.0.2", "v1.0.0"]);
  });

  test("non-version strings sort before semver versions", () => {
    const sorted = ["v1.0.0", "draft", "v0.99.0"].sort(compareVersionsDesc);
    expect(sorted[0]).toBe("draft");
  });
});

describe("isReleased", () => {
  test("returns true when version is at or below latest", () => {
    expect(isReleased("v1.0.3", "1.0.3")).toBe(true);
    expect(isReleased("v1.0.0", "1.0.3")).toBe(true);
  });

  test("returns false when version is newer than latest", () => {
    expect(isReleased("v1.0.4", "1.0.3")).toBe(false);
  });

  test("returns false for non-semver tags", () => {
    expect(isReleased("draft", "1.0.3")).toBe(false);
  });
});

describe("categorySlugSort", () => {
  test("uses the canonical category order", () => {
    const slugs = ["bug-fixes", "breaking-changes", "new-features"].sort(categorySlugSort);
    expect(slugs).toEqual(["breaking-changes", "new-features", "bug-fixes"]);
  });
});

describe("parsePullRequests", () => {
  test("returns empty array on null body", () => {
    expect(parsePullRequests(null)).toEqual([]);
  });

  test("groups items under their conventional type and stops at next h2", () => {
    const body = [
      "## What's Changed",
      "* feat: add a thing by @alice in https://github.com/o/r/pull/1",
      "* fix(parser): tighten regex by @bob in https://github.com/o/r/pull/2",
      "* feat!: rip out the old API by @carol in https://github.com/o/r/pull/3",
      "* docs: tidy README by @dan in https://github.com/o/r/pull/4",
      "",
      "## New Contributors",
      "* should not be parsed by @eve in https://github.com/o/r/pull/99",
    ].join("\n");

    const groups = parsePullRequests(body);
    const labels = groups.map((g) => g.type.key);

    expect(labels).toEqual(["breaking", "feat", "fix", "docs"]);
    expect(groups[1].items[0].prNumber).toBe(1);
    expect(groups[0].items[0].prNumber).toBe(3);
    expect(groups.flatMap((g) => g.items.map((i) => i.author))).not.toContain("eve");
  });

  test("ignores text outside the What's Changed section", () => {
    const body = [
      "Intro paragraph",
      "* fix: should not be picked up by @x in https://github.com/o/r/pull/1",
      "## What's Changed",
      "* feat: real entry by @y in https://github.com/o/r/pull/2",
    ].join("\n");

    const groups = parsePullRequests(body);
    expect(groups).toHaveLength(1);
    expect(groups[0].items[0].author).toBe("y");
  });
});

describe("pullRequestsToMarkdown", () => {
  test("formats grouped items with author and PR links", () => {
    const md = pullRequestsToMarkdown(
      parsePullRequests(
        [
          "## What's Changed",
          "* feat: add a thing by @alice in https://github.com/o/r/pull/1",
        ].join("\n"),
      ),
    );

    expect(md).toContain("## Pull Requests");
    expect(md).toContain("### ✨ Features");
    expect(md).toContain("[@alice](https://github.com/alice)");
    expect(md).toContain("[#1](https://github.com/o/r/pull/1)");
  });
});

describe("prepareForGitHub", () => {
  test("rewrites root-relative links to absolute URLs", () => {
    const out = prepareForGitHub("See [the docs](/features/x).", SITE);
    expect(out).toBe(`See [the docs](${SITE}/features/x).`);
  });

  test("flattens entry and category headings to h2 and strips horizontal rules", () => {
    // The wrapping page renders categories as `### Label` and entries as
    // `#### Title`. The two-step rewrite below collapses both to `##`, since
    // pass two also catches the `###` produced by pass one. GitHub releases
    // render `##` as the largest distinguishable section heading.
    const out = prepareForGitHub(
      ["### ✨ New Features", "", "---", "", "#### Did a thing", "", "Body."].join("\n"),
      SITE,
    );
    expect(out).toBe(["## ✨ New Features", "", "## Did a thing", "", "Body."].join("\n"));
  });

  test("strips MDX import lines", () => {
    const out = prepareForGitHub(
      [
        "import { Aside } from '@astrojs/starlight/components'",
        "",
        "#### Title",
        "",
        "Body.",
      ].join("\n"),
      SITE,
    );
    expect(out).toBe(["## Title", "", "Body."].join("\n"));
  });

  test("converts a tip Aside with a title to a GitHub TIP alert", () => {
    const out = prepareForGitHub(
      ['<Aside type="tip" title="Heads up">', "Be aware.", "</Aside>"].join("\n"),
      SITE,
    );
    expect(out).toBe(["> [!TIP]", "> **Heads up**", ">", "> Be aware."].join("\n"));
  });

  test("uses NOTE for default Aside type and omits title line when absent", () => {
    const out = prepareForGitHub("<Aside>Just a note.</Aside>", SITE);
    expect(out).toBe(["> [!NOTE]", "> Just a note."].join("\n"));
  });

  test("maps Starlight caution and danger to WARNING and CAUTION", () => {
    const caution = prepareForGitHub('<Aside type="caution">be careful</Aside>', SITE);
    const danger = prepareForGitHub('<Aside type="danger">stop</Aside>', SITE);
    expect(caution).toContain("> [!WARNING]");
    expect(danger).toContain("> [!CAUTION]");
  });

  test("transforms multiple Asides independently", () => {
    const out = prepareForGitHub(
      ['<Aside type="tip">first</Aside>', "", '<Aside type="caution">second</Aside>'].join("\n"),
      SITE,
    );
    expect(out).toContain("> [!TIP]");
    expect(out).toContain("> first");
    expect(out).toContain("> [!WARNING]");
    expect(out).toContain("> second");
  });

  test("preserves blank lines inside an Aside body as `>` continuation rows", () => {
    const out = prepareForGitHub(
      [
        '<Aside type="note" title="Heads up">',
        "para one",
        "",
        "para two",
        "</Aside>",
      ].join("\n"),
      SITE,
    );
    expect(out).toContain("> para one");
    expect(out).toContain(">\n> para two");
  });

  test("collapses runs of blank lines to a single blank line", () => {
    const out = prepareForGitHub("a\n\n\n\nb", SITE);
    expect(out).toBe("a\n\nb");
  });
});
