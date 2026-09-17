import { describe, test, expect } from "bun:test";
import path from "node:path";
import { checkBunPins, run } from "./vercel-bun-sync";

const IN_SYNC = {
  miseToml: '[tools]\nbun = "1.3.0"\n',
  vercelConfig: {
    installCommand: "bunx bun@1.3.0 install",
    buildCommand: "bunx bun@1.3.0 run build",
  },
};

describe("checkBunPins", () => {
  test("passes when every build command matches the mise pin", () => {
    expect(checkBunPins(IN_SYNC)).toEqual([]);
  });

  test("reports a mismatch per command", () => {
    const failures = checkBunPins({
      miseToml: '[tools]\nbun = "1.3.0"\n',
      vercelConfig: {
        installCommand: "bunx bun@1.2.0 install",
        buildCommand: "bunx bun@1.4.0 run build",
      },
    });
    expect(failures).toEqual([
      "installCommand runs bun@1.2.0 but mise.toml pins 1.3.0",
      "buildCommand runs bun@1.4.0 but mise.toml pins 1.3.0",
    ]);
  });

  test("reports a missing command", () => {
    const failures = checkBunPins({
      miseToml: IN_SYNC.miseToml,
      vercelConfig: { buildCommand: IN_SYNC.vercelConfig.buildCommand },
    });
    expect(failures).toEqual([
      "installCommand is missing; nothing pins the build's bun version",
    ]);
  });

  test("reports a command that runs bun without a version pin", () => {
    const failures = checkBunPins({
      miseToml: IN_SYNC.miseToml,
      vercelConfig: {
        installCommand: "bun install",
        buildCommand: "bun run build",
      },
    });
    expect(failures).toEqual([
      "installCommand does not name a bun version to run",
      "buildCommand does not name a bun version to run",
    ]);
  });

  test("reports a mise.toml without a bun pin", () => {
    const failures = checkBunPins({
      miseToml: '[tools]\nnode = "20.0.0"\n',
      vercelConfig: IN_SYNC.vercelConfig,
    });
    expect(failures).toEqual(["mise.toml does not pin a bun version"]);
  });

  test("reports an unparseable mise.toml", () => {
    const failures = checkBunPins({
      miseToml: "[tools\nbun =",
      vercelConfig: IN_SYNC.vercelConfig,
    });
    expect(failures).toHaveLength(1);
    expect(failures[0]).toContain("mise.toml is unparseable");
  });

  test("reports an unreadable mise.toml over checking either file", () => {
    const failures = checkBunPins({
      miseToml: IN_SYNC.miseToml,
      vercelConfig: IN_SYNC.vercelConfig,
      miseError: "no such file or directory",
      vercelError: "no such file or directory",
    });
    expect(failures).toEqual([
      "mise.toml is unreadable: no such file or directory",
    ]);
  });

  test("reports an unreadable vercel.json when mise.toml reads", () => {
    const failures = checkBunPins({
      miseToml: IN_SYNC.miseToml,
      vercelConfig: IN_SYNC.vercelConfig,
      vercelError: "Unexpected token in JSON",
    });
    expect(failures).toEqual([
      "docs/vercel.json is unreadable: Unexpected token in JSON",
    ]);
  });
});

describe("run", () => {
  const repoRoot = path.join(import.meta.dir, "..", "..");

  test("the checked-in mise.toml and docs/vercel.json pins agree", () => {
    expect(
      run({
        misePath: path.join(repoRoot, "mise.toml"),
        vercelPath: path.join(repoRoot, "docs", "vercel.json"),
      }),
    ).toEqual([]);
  });

  test("reports read failures instead of crashing", () => {
    const failures = run({
      misePath: "missing/mise.toml",
      vercelPath: "missing/vercel.json",
      fs: {
        readFileSync: () => {
          throw new Error("no such file or directory");
        },
      },
    });
    expect(failures).toHaveLength(1);
    expect(failures[0]).toContain("mise.toml is unreadable");
  });
});
