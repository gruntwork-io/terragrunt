/**
 * Asserts that the Vercel docs build runs the bun that mise.toml pins.
 */
import fs from "node:fs";

// The vercel.json commands that build the site. Anything else in the file
// runs after the build, when the bun version no longer matters.
const BUILD_COMMANDS = ["installCommand", "buildCommand"];

type Filesystem = {
  readFileSync(path: string, encoding: BufferEncoding): string;
};

/**
 * Extracts the bun pin from a mise.toml source.
 *
 * Returns null when mise.toml does not pin a bun version. Throws when the
 * source is not valid TOML.
 */
export function miseBunPin(source: string): string | null {
  const mise = Bun.TOML.parse(source) as { tools?: { bun?: unknown } };
  const pin = mise.tools?.bun;
  return typeof pin === "string" ? pin : null;
}

/**
 * Extracts the version of the bun binary a command runs, or null when the
 * command does not run a pinned bun.
 */
export function commandBunPin(command: unknown): string | null {
  if (typeof command !== "string") {
    return null;
  }
  const match = command.match(/bunx\s+bun@([^\s"]+)/);
  return match ? match[1] : null;
}

/**
 * Checks that vercel.json's install and build commands run the bun that
 * mise.toml pins.
 *
 * Returns one message per violation, empty when the pins agree.
 */
export function checkBunPins({
  miseToml,
  vercelConfig,
  miseError,
  vercelError,
}: {
  miseToml: string;
  vercelConfig: Record<string, unknown>;
  miseError?: string | null;
  vercelError?: string | null;
}): string[] {
  const failures: string[] = [];

  // An unreadable file can still hold the truth on disk; report the read
  // failure instead of checking against a partial picture of either side.
  if (miseError) {
    failures.push(`mise.toml is unreadable: ${miseError}`);
  } else if (vercelError) {
    failures.push(`docs/vercel.json is unreadable: ${vercelError}`);
  } else {
    let misePin: string | null = null;
    let parseFailed = false;
    try {
      misePin = miseBunPin(miseToml);
    } catch (error) {
      failures.push(`mise.toml is unparseable: ${error instanceof Error ? error.message : String(error)}`);
      parseFailed = true;
    }
    if (!parseFailed && !misePin) {
      failures.push("mise.toml does not pin a bun version");
    }

    if (misePin) {
      for (const command of BUILD_COMMANDS) {
        const value = vercelConfig[command];
        if (value === undefined) {
          failures.push(
            `${command} is missing; nothing pins the build's bun version`,
          );
          continue;
        }
        const pinned = commandBunPin(value);
        if (!pinned) {
          failures.push(`${command} does not name a bun version to run`);
        } else if (pinned !== misePin) {
          failures.push(
            `${command} runs bun@${pinned} but mise.toml pins ${misePin}`,
          );
        }
      }
    }
  }

  return failures;
}

/**
 * Reads both files from disk and checks their bun pins.
 */
export function run({
  misePath,
  vercelPath,
  fs: fsImpl = fs,
}: {
  misePath: string;
  vercelPath: string;
  fs?: Filesystem;
}): string[] {
  let miseToml: string;
  let miseError: string | null = null;
  let vercelConfig: Record<string, unknown>;
  let vercelError: string | null = null;

  try {
    miseToml = fsImpl.readFileSync(misePath, "utf8");
  } catch (error) {
    miseError = error instanceof Error ? error.message : String(error);
    miseToml = "";
  }
  try {
    vercelConfig = JSON.parse(fsImpl.readFileSync(vercelPath, "utf8")) as Record<string, unknown>;
  } catch (error) {
    vercelError = error instanceof Error ? error.message : String(error);
    vercelConfig = {};
  }

  return checkBunPins({ miseToml, vercelConfig, miseError, vercelError });
}
