import { describe, expect, test } from "bun:test";
import {
  isCompleted,
  isVersionReleased,
  type ExperimentEntry,
} from "./experiments";

function makeEntry(
  data: Partial<ExperimentEntry["data"]> & { name: string },
): ExperimentEntry {
  return {
    id: data.name,
    collection: "experiments",
    data,
  } as ExperimentEntry;
}

describe("isVersionReleased", () => {
  test("treats an undefined version as released", () => {
    expect(isVersionReleased(undefined, "1.0.7")).toBe(true);
  });

  test("returns false when the version is newer than the latest release", () => {
    expect(isVersionReleased("1.1", "1.0.7")).toBe(false);
  });

  test("returns true once the latest release reaches the version", () => {
    expect(isVersionReleased("1.1", "1.1.0")).toBe(true);
  });

  test("tolerates a leading v prefix on either side", () => {
    expect(isVersionReleased("v1.0.0", "1.0.7")).toBe(true);
  });

  test("compares numerically not lexically (1.0.10 > 1.0.7)", () => {
    expect(isVersionReleased("1.0.10", "1.0.7")).toBe(false);
    expect(isVersionReleased("1.0.7", "1.0.10")).toBe(true);
  });
});

describe("isCompleted", () => {
  test("is false when the field is absent", () => {
    expect(isCompleted(makeEntry({ name: "symlinks", since: "1.0.1" }), "1.0.7")).toBe(false);
  });

  test("is false while the completion release is unreleased", () => {
    const entry = makeEntry({ name: "cas", completedSince: "1.1" });
    expect(isCompleted(entry, "1.0.7")).toBe(false);
  });

  test("is true once the completion release has shipped", () => {
    const entry = makeEntry({ name: "cas", completedSince: "1.1" });
    expect(isCompleted(entry, "1.1.0")).toBe(true);
  });

  test("treats a v-prefixed completion release the same as an unprefixed one", () => {
    const entry = makeEntry({ name: "report", completedSince: "v1.0.0" });
    expect(isCompleted(entry, "1.0.7")).toBe(true);
  });
});
