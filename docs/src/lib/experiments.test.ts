import { describe, expect, test } from "bun:test";
import { isCompleted, type ExperimentEntry } from "./experiments";

function makeEntry(
  data: Partial<ExperimentEntry["data"]> & { name: string },
): ExperimentEntry {
  return {
    id: data.name,
    collection: "experiments",
    data,
  } as ExperimentEntry;
}

describe("isCompleted", () => {
  test("is false when the field is absent", () => {
    expect(isCompleted(makeEntry({ name: "symlinks", since: "v1.0.1" }), "v1.0.7")).toBe(false);
  });

  test("is false while the completion release is unreleased", () => {
    const entry = makeEntry({ name: "cas", completedSince: "v1.1.0" });
    expect(isCompleted(entry, "v1.0.7")).toBe(false);
  });

  test("is true once the completion release has shipped", () => {
    const entry = makeEntry({ name: "cas", completedSince: "v1.1.0" });
    expect(isCompleted(entry, "v1.1.0")).toBe(true);
  });
});
