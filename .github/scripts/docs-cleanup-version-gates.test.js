const { describe, test, expect } = require("bun:test");
const {
  isReleased,
  cleanupContent,
  pruneImports,
} = require("./docs-cleanup-version-gates");

const IMPORTS = [
  "import Since from '@components/Since.astro';",
  "import Before from '@components/Before.astro';",
].join("\n");

describe("isReleased", () => {
  test("treats an equal version as released", () => {
    expect(isReleased("v1.0.3", "1.0.3")).toBe(true);
  });

  test("treats a lower target as released", () => {
    expect(isReleased("v1.0.5", "1.0.3")).toBe(true);
  });

  test("treats a higher target as unreleased", () => {
    expect(isReleased("v1.0.3", "1.0.5")).toBe(false);
  });

  test("ignores a leading v on either side", () => {
    expect(isReleased("1.0.3", "v1.0.3")).toBe(true);
    expect(isReleased("v1.0.3", "1.0.4")).toBe(false);
  });

  test("orders multi-digit components numerically", () => {
    expect(isReleased("v1.0.10", "1.0.2")).toBe(true);
    expect(isReleased("v1.0.2", "1.0.10")).toBe(false);
  });
});

describe("cleanupContent inline form", () => {
  test("drops Before and unwraps Since on a shared line", () => {
    const source =
      'The path is <Before version="1.0.7">`old`</Before><Since version="1.0.7">`new`</Since>. Done.';
    const { content, changed } = cleanupContent(source, "v1.0.7");
    expect(changed).toBe(true);
    expect(content).toBe("The path is `new`. Done.");
  });
});

describe("cleanupContent block form", () => {
  test("removes a Before block and unwraps a Since block with nested Aside", () => {
    const source = [
      IMPORTS,
      "",
      "## Heading",
      "",
      '<Before version="1.0.5">',
      "",
      '<Aside type="tip">Coming soon.</Aside>',
      "",
      "</Before>",
      "",
      '<Since version="1.0.5">',
      "",
      "- the new attribute",
      "",
      "</Since>",
      "",
      "Trailing text.",
      "",
    ].join("\n");

    const { content, changed } = cleanupContent(source, "v1.0.5");
    expect(changed).toBe(true);
    expect(content).not.toContain("<Before");
    expect(content).not.toContain("<Since");
    expect(content).not.toContain("Coming soon.");
    expect(content).toContain("- the new attribute");
    expect(content).not.toContain("import Before");
    expect(content).not.toContain("import Since");
    expect(content).not.toMatch(/\n{3,}/);
  });
});

describe("cleanupContent nesting", () => {
  const nested = [
    IMPORTS,
    "",
    '<Since version="1.0.3">',
    "",
    "Outer body.",
    "",
    '<Before version="1.0.5">',
    "Coming in 1.0.5.",
    "</Before>",
    "",
    '<Since version="1.0.5">',
    "Shipped in 1.0.5.",
    "</Since>",
    "",
    "</Since>",
    "",
  ].join("\n");

  test("unwraps the outer gate while preserving inner unreleased gates", () => {
    const { content, changed } = cleanupContent(nested, "v1.0.3");
    expect(changed).toBe(true);
    expect(content).toContain("Outer body.");
    // The 1.0.5 gates are not yet released, so they stay wrapped.
    expect(content).toContain('<Before version="1.0.5">');
    expect(content).toContain('<Since version="1.0.5">');
    // Imports stay because the inner gates still use both components.
    expect(content).toContain("import Before");
    expect(content).toContain("import Since");
  });

  test("resolves every nested gate once all versions have shipped", () => {
    const { content } = cleanupContent(nested, "v1.0.5");
    expect(content).not.toContain("<Before");
    expect(content).not.toContain("<Since");
    expect(content).not.toContain("Coming in 1.0.5.");
    expect(content).toContain("Outer body.");
    expect(content).toContain("Shipped in 1.0.5.");
    expect(content).not.toContain("import Before");
    expect(content).not.toContain("import Since");
  });
});

describe("cleanupContent mixed versions", () => {
  test("only touches gates at or below the latest release", () => {
    const source = [
      IMPORTS,
      "",
      '<Before version="1.0.3">old behavior</Before>',
      '<Since version="1.0.3">new behavior</Since>',
      "",
      '<Before version="2.0.0">future old</Before>',
      '<Since version="2.0.0">future new</Since>',
      "",
    ].join("\n");

    const { content, changed } = cleanupContent(source, "v1.0.3");
    expect(changed).toBe(true);
    // Released gates resolved.
    expect(content).not.toContain('version="1.0.3"');
    expect(content).toContain("new behavior");
    expect(content).not.toContain("old behavior");
    // Unreleased gates untouched, so both imports remain.
    expect(content).toContain('<Before version="2.0.0">future old</Before>');
    expect(content).toContain('<Since version="2.0.0">future new</Since>');
    expect(content).toContain("import Before");
    expect(content).toContain("import Since");
  });
});

describe("pruneImports", () => {
  test("drops only the fully unused component import", () => {
    const source = [
      IMPORTS,
      "",
      '<Since version="2.0.0">still gated</Since>',
      "",
    ].join("\n");
    const result = pruneImports(source);
    expect(result).not.toContain("import Before");
    expect(result).toContain("import Since");
  });
});

describe("cleanupContent idempotency", () => {
  test("a second pass produces identical output", () => {
    const source = [
      IMPORTS,
      "",
      '<Before version="1.0.3">old</Before><Since version="1.0.3">new</Since>',
      "",
      '<Since version="2.0.0">future</Since>',
      "",
    ].join("\n");

    const first = cleanupContent(source, "v1.0.3");
    const second = cleanupContent(first.content, "v1.0.3");
    expect(second.changed).toBe(false);
    expect(second.content).toBe(first.content);
  });
});

describe("cleanupContent leaves unbalanced input untouched", () => {
  test("reports an error and makes no changes", () => {
    const source = '<Since version="1.0.3">missing close';
    const { content, changed, error } = cleanupContent(source, "v1.0.3");
    expect(changed).toBe(false);
    expect(content).toBe(source);
    expect(error).toBeDefined();
  });
});
