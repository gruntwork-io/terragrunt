import { describe, expect, test } from "bun:test";
import { isCompleted, type StrictControlEntry } from "./strict-controls";

function makeEntry(
  data: Partial<StrictControlEntry["data"]> & { name: string },
): StrictControlEntry {
  return {
    id: data.name,
    collection: "strictControls",
    data,
  } as StrictControlEntry;
}

describe("isCompleted", () => {
  test("is false when the field is absent", () => {
    expect(isCompleted(makeEntry({ name: "fast-copy", since: "v1.0.4" }), "v1.1.6")).toBe(false);
  });

  test("is false while the completion release is unreleased", () => {
    const entry = makeEntry({ name: "legacy-base64gzip", since: "v1.1.5", completedSince: "v1.2.0" });
    expect(isCompleted(entry, "v1.1.6")).toBe(false);
  });

  test("is true once the completion release has shipped", () => {
    const entry = makeEntry({ name: "legacy-base64gzip", since: "v1.1.5", completedSince: "v1.2.0" });
    expect(isCompleted(entry, "v1.2.0")).toBe(true);
  });
});
