import { describe, expect, test } from "bun:test";
import {
  effectiveTerragruntMax,
  filterByReleasedMin,
  type CompatibilityEntry,
} from "./compatibility";

function makeEntry(id: string, terragrunt_min: string): CompatibilityEntry {
  return {
    id,
    collection: "compatibility",
    data: {
      id,
      tool: "opentofu",
      version: "1.0.x",
      terragrunt_min,
      terragrunt_max: null,
      order: 1,
    },
  } as CompatibilityEntry;
}

describe("filterByReleasedMin", () => {
  test("keeps entries whose terragrunt_min is at or below the latest release", () => {
    const entries = [makeEntry("a", "1.0.0"), makeEntry("b", "1.0.5")];
    const result = filterByReleasedMin(entries, "1.0.5");
    expect(result.map((e) => e.id)).toEqual(["a", "b"]);
  });

  test("drops entries whose terragrunt_min is newer than the latest release", () => {
    const entries = [makeEntry("released", "1.0.5"), makeEntry("future", "1.1.0")];
    const result = filterByReleasedMin(entries, "1.0.5");
    expect(result.map((e) => e.id)).toEqual(["released"]);
  });

  test("compares numerically not lexically (1.0.10 > 1.0.5)", () => {
    const entries = [makeEntry("a", "1.0.10")];
    expect(filterByReleasedMin(entries, "1.0.5")).toEqual([]);
    expect(filterByReleasedMin(entries, "1.0.10").map((e) => e.id)).toEqual(["a"]);
  });

  test("returns all entries when showUnreleased is true", () => {
    const entries = [makeEntry("a", "1.0.0"), makeEntry("future", "99.0.0")];
    const result = filterByReleasedMin(entries, "1.0.0", true);
    expect(result.map((e) => e.id)).toEqual(["a", "future"]);
  });

  test("returns empty array when latest is 0.0.0 (treats nothing as released)", () => {
    const entries = [makeEntry("a", "1.0.0")];
    expect(filterByReleasedMin(entries, "0.0.0")).toEqual([]);
  });
});

describe("effectiveTerragruntMax", () => {
  test("returns null when input is already null", () => {
    expect(effectiveTerragruntMax(null, "1.0.5")).toBe(null);
  });

  test("returns the value when it is at or below the latest release", () => {
    expect(effectiveTerragruntMax("1.0.5", "1.0.5")).toBe("1.0.5");
    expect(effectiveTerragruntMax("0.24.4", "1.0.5")).toBe("0.24.4");
  });

  test("returns null when the value is newer than the latest release", () => {
    expect(effectiveTerragruntMax("1.1.0", "1.0.5")).toBe(null);
  });

  test("compares numerically not lexically", () => {
    expect(effectiveTerragruntMax("1.0.10", "1.0.5")).toBe(null);
    expect(effectiveTerragruntMax("1.0.10", "1.0.10")).toBe("1.0.10");
  });

  test("returns the original value unchanged when showUnreleased is true", () => {
    expect(effectiveTerragruntMax("99.0.0", "1.0.5", true)).toBe("99.0.0");
  });
});
