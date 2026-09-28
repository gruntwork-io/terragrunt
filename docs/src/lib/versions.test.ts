import { describe, expect, test } from "bun:test";
import {
  RELEASE_VERSION,
  assertReleaseVersion,
  compareVersionsDesc,
  isReleased,
} from "./versions";

describe("RELEASE_VERSION", () => {
  test("matches a v-prefixed three-part version", () => {
    expect(RELEASE_VERSION.test("v1.2.0")).toBe(true);
    expect(RELEASE_VERSION.test("v0.99.10")).toBe(true);
  });

  test("rejects other forms", () => {
    for (const version of ["1.2.0", "v1.2", "v1.2.0-rc1", "V1.2.0", "draft", " v1.2.0"]) {
      expect(RELEASE_VERSION.test(version)).toBe(false);
    }
  });
});

describe("assertReleaseVersion", () => {
  test("accepts a vX.Y.Z version", () => {
    expect(() => assertReleaseVersion("v1.2.0")).not.toThrow();
  });

  test("throws on a version without the v prefix", () => {
    expect(() => assertReleaseVersion("1.2.0")).toThrow('version "1.2.0" must have the form vX.Y.Z');
  });
});

describe("isReleased", () => {
  test("returns true when the version is at or below the latest release", () => {
    expect(isReleased("v1.0.3", "v1.0.3")).toBe(true);
    expect(isReleased("v1.0.0", "v1.0.3")).toBe(true);
  });

  test("returns false when the version is newer than the latest release", () => {
    expect(isReleased("v1.0.4", "v1.0.3")).toBe(false);
  });

  test("compares numerically, not lexically", () => {
    expect(isReleased("v1.0.10", "v1.0.7")).toBe(false);
    expect(isReleased("v1.0.7", "v1.0.10")).toBe(true);
  });

  test("treats every version as unreleased against v0.0.0", () => {
    expect(isReleased("v0.0.1", "v0.0.0")).toBe(false);
  });
});

describe("compareVersionsDesc", () => {
  test("orders versions newest first", () => {
    const sorted = ["v1.0.2", "v1.0.10", "v0.99.0", "v1.0.0"].sort(compareVersionsDesc);
    expect(sorted).toEqual(["v1.0.10", "v1.0.2", "v1.0.0", "v0.99.0"]);
  });
});
