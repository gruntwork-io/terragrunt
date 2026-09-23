/**
 * Removes documentation version gates that a release has made obsolete.
 *
 * The docs site gates unreleased content behind two Astro components:
 *
 *   - <Before version="X">…</Before> renders only while the latest release is
 *     older than X (the "current behavior" shown until X ships).
 *   - <Since version="X">…</Since> renders only once the latest release is >= X
 *     (the "new behavior" shown after X ships).
 *
 * Once a version ships, its gates are dead weight: a <Before> block can never
 * render again, and a <Since> block is now permanent content needlessly wrapped
 * in a conditional. After a release, this script rewrites every .mdx file under
 * the docs content tree, for every gate whose version is at or below the latest
 * release:
 *
 *   - <Before version="X">…</Before>  ->  removed entirely (content included).
 *   - <Since version="X">…</Since>    ->  unwrapped (the gate is dropped, the
 *                                         content is kept).
 *
 * Gates whose version is above the latest release are left untouched, including
 * when they are nested inside a gate that does get unwrapped. Once a component
 * has no remaining usages in a file, its import line is dropped.
 *
 * The script takes no network calls; the workflow passes the latest release tag
 * in via LATEST_TAG.
 */
import fs from "node:fs";
import path from "node:path";
import type { Dirent } from "node:fs";

type Filesystem = {
  readFileSync(path: string, encoding: BufferEncoding): string;
  writeFileSync(path: string, data: string): void;
  readdirSync(path: string, options: { withFileTypes: true }): Dirent[];
};

// Matches either an opening gate (<Before version="X"> / <Since version="X">)
// or a closing gate (</Before> / </Since>). Group 1/2 are the tag/version of an
// opening tag; group 3 is the tag of a closing tag.
const GATE_TOKEN =
  /<(Before|Since)\s+version=["']([^"']+)["']\s*>|<\/(Before|Since)>/g;

type GateTag = "Before" | "Since";

type TextNode = { type: "text"; value: string };

type GateNode = {
  type: "gate";
  tag: GateTag;
  version: string;
  children: Node[];
  start: number;
  end: number;
  raw: string;
};

type Node = TextNode | GateNode;

// The root the parse builds into. It has the shape of an unreleased gate, so
// the stack can treat every frame the same; it never leaves the stack.
type StackNode = GateNode | { type: "root"; children: Node[] };

// Placeholder left where a released gate is removed, so the whitespace it
// occupied can be reclaimed without disturbing blank lines elsewhere in the
// file. A NUL byte never appears in the source.
const REMOVED = String.fromCharCode(0);

/**
 * Reports whether `targetVersion` is at or below `latestTag`, i.e. the version
 * has shipped. This intentionally mirrors the comparison used by Before.astro,
 * Since.astro, and isReleased() in docs/src/lib/changelog.ts. Keep the three in
 * sync: a leading "v" is stripped from both sides and the remainder is compared
 * with locale-aware numeric ordering so v1.0.10 sorts after v1.0.2.
 */
export function isReleased(latestTag: string, targetVersion: string): boolean {
  const latest = String(latestTag).replace(/^v/, "");
  const target = String(targetVersion).replace(/^v/, "");
  return latest.localeCompare(target, undefined, { numeric: true }) >= 0;
}

/**
 * Parses `source` into a flat list of nodes, where each node is either a text
 * run or a gate with its own child nodes. Gates nest, so this builds a tree
 * rather than matching tags with a flat regex.
 *
 * Throws when the gate tags are unbalanced.
 */
export function parseNodes(source: string): Node[] {
  const root: StackNode = { type: "root", children: [] };
  const stack: StackNode[] = [root];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  GATE_TOKEN.lastIndex = 0;
  while ((match = GATE_TOKEN.exec(source)) !== null) {
    const top = stack[stack.length - 1];
    if (match.index > lastIndex) {
      top.children.push({
        type: "text",
        value: source.slice(lastIndex, match.index),
      });
    }

    const isOpening = match[1] !== undefined;
    if (isOpening) {
      const node: GateNode = {
        type: "gate",
        tag: match[1] as GateTag,
        version: match[2],
        children: [],
        start: match.index,
        end: 0,
        raw: "",
      };
      top.children.push(node);
      stack.push(node);
    } else {
      const node = stack.pop();
      if (!node || node.type !== "gate" || node.tag !== match[3]) {
        throw new Error(`Unbalanced </${match[3]}> tag`);
      }
      node.end = match.index + match[0].length;
      node.raw = source.slice(node.start, node.end);
    }

    lastIndex = GATE_TOKEN.lastIndex;
  }

  if (lastIndex < source.length) {
    stack[stack.length - 1].children.push({
      type: "text",
      value: source.slice(lastIndex),
    });
  }

  if (stack.length !== 1) {
    throw new Error("Unbalanced gate tags");
  }
  return root.children;
}

/**
 * Renders a node list back to source, applying the cleanup rules against
 * `latestTag`. Children are always cleaned first, so a released gate nested
 * inside an unreleased one is still resolved. Unchanged subtrees are emitted
 * verbatim from their original source to keep diffs minimal. Removed gates
 * leave a REMOVED sentinel that cleanupContent later reclaims.
 */
function renderNodes(
  nodes: Node[],
  latestTag: string,
): { text: string; changed: boolean } {
  let text = "";
  let changed = false;

  for (const node of nodes) {
    if (node.type === "text") {
      text += node.value;
      continue;
    }

    const child = renderNodes(node.children, latestTag);

    if (isReleased(latestTag, node.version)) {
      // Before -> drop content; Since -> keep content, drop the gate. Either
      // way the gate's own span collapses to a sentinel.
      text += node.tag === "Since" ? REMOVED + child.text + REMOVED : REMOVED;
      changed = true;
      continue;
    }

    // The gate stays. Re-emit verbatim when nothing inside it changed.
    if (child.changed) {
      text += `<${node.tag} version="${node.version}">${child.text}</${node.tag}>`;
      changed = true;
    } else {
      text += node.raw;
    }
  }

  return { text, changed };
}

/**
 * Drops the import line for a gate component once the file has no remaining
 * usages of it.
 */
export function pruneImports(source: string): string {
  let result = source;
  for (const tag of ["Before", "Since"]) {
    if (new RegExp(`<${tag}\\b`).test(result)) {
      continue;
    }
    const importLine = new RegExp(
      `^import ${tag} from ['"]@components/${tag}\\.astro['"];?\\n`,
      "m",
    );
    result = result.replace(importLine, "");
  }
  return result;
}

/**
 * Reclaims the whitespace around removed gates, working line by line so blank
 * lines elsewhere in the file are left untouched.
 *
 * A block-form gate sits on its own line between blank lines, so removing it
 * leaves a sentinel alone on a line (a "block sentinel"). A run of such lines,
 * together with any blank lines flanking it, collapses to a single blank-line
 * separator — or to nothing at the start or end of the file. A sentinel sharing
 * a line with real text is the inline form and is simply deleted.
 */
function reclaimWhitespace(source: string): string {
  const lines = source.split("\n");
  const out: string[] = [];

  for (let i = 0; i < lines.length; ) {
    const line = lines[i];
    const withoutSentinels = line.split(REMOVED).join("");
    const isBlockSentinel =
      line.includes(REMOVED) && withoutSentinels.trim() === "";

    if (!isBlockSentinel) {
      out.push(withoutSentinels);
      i++;
      continue;
    }

    // Drop blank lines already emitted just before this run, consume the run of
    // block-sentinel and blank lines, then re-separate the surrounding content
    // with a single blank line (unless the run sits at a file boundary).
    while (out.length > 0 && out[out.length - 1] === "") {
      out.pop();
    }
    while (i < lines.length) {
      const current = lines[i];
      const stripped = current.split(REMOVED).join("").trim();
      const blockSentinel = current.includes(REMOVED) && stripped === "";
      if (blockSentinel || stripped === "") {
        i++;
        continue;
      }
      break;
    }
    if (out.length > 0 && i < lines.length) {
      out.push("");
    }
  }

  return out.join("\n");
}

/**
 * Rewrites a single file's contents, removing gates the release has obsoleted.
 */
export function cleanupContent(
  source: string,
  latestTag: string,
): { content: string; changed: boolean; error?: string } {
  let nodes: Node[];
  try {
    nodes = parseNodes(source);
  } catch (error) {
    return {
      content: source,
      changed: false,
      error: error instanceof Error ? error.message : String(error),
    };
  }

  const { text, changed } = renderNodes(nodes, latestTag);
  if (!changed) {
    return { content: source, changed: false };
  }

  const content = pruneImports(reclaimWhitespace(text));
  return { content, changed: content !== source };
}

/**
 * Recursively collects every .mdx file under `dir`.
 */
export function listMdxFiles(dir: string, fsImpl: Filesystem): string[] {
  const files: string[] = [];
  for (const entry of fsImpl.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listMdxFiles(full, fsImpl));
      continue;
    }
    if (entry.isFile() && full.endsWith(".mdx")) {
      files.push(full);
    }
  }
  return files;
}

/**
 * Walks the docs content tree and rewrites every file with obsolete gates.
 *
 * Returns the paths of the files that changed.
 */
export function run({
  contentDir,
  latestTag,
  fs: fsImpl = fs,
  log = console.log,
}: {
  contentDir: string;
  latestTag: string;
  fs?: Filesystem;
  log?: (message: string) => void;
}): string[] {
  const changedFiles: string[] = [];
  for (const file of listMdxFiles(contentDir, fsImpl)) {
    const source = fsImpl.readFileSync(file, "utf8");
    if (!source.includes("<Before") && !source.includes("<Since")) {
      continue;
    }

    const { content, changed, error } = cleanupContent(source, latestTag);
    if (error) {
      log(`Skipping ${file}: ${error}`);
      continue;
    }
    if (!changed) {
      continue;
    }

    fsImpl.writeFileSync(file, content);
    changedFiles.push(file);
    log(`Cleaned ${file}`);
  }
  return changedFiles;
}

if (import.meta.main) {
  const latestTag = process.env.LATEST_TAG || process.argv[2];
  const contentDir =
    process.env.CONTENT_DIR ||
    process.argv[3] ||
    path.join(import.meta.dir, "..", "..", "docs", "src", "content");

  if (!latestTag) {
    console.error("LATEST_TAG is required.");
    process.exit(1);
  }

  const changed = run({ contentDir, latestTag });
  console.log(`Cleaned ${changed.length} file(s) for ${latestTag}.`);
}