/**
 * Assert every built page carries the Google tag, pointed at our own origin.
 *
 * A route that legitimately has no tag reads identically to one that lost it,
 * so this requires the tag everywhere rather than warning about absences. The
 * failure it guards is total and silent: analytics stops, and no page looks any
 * different to anyone reviewing the change.
 *
 * It also fails on a tag pointing straight at googletagmanager.com. That works
 * — which is the problem. It is the exact state the first-party setup exists to
 * prevent, and nothing else would notice.
 */
import { readdirSync, existsSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

const DIST = "dist";
const EXPECTED = '"/mtag/gtm.js?id="';
const DIRECT = "googletagmanager.com";

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else if (entry.name.endsWith(".html")) out.push(full);
  }
  return out;
}

if (!existsSync(DIST)) {
  console.error(
    `analytics:check — no build at ./${DIST}. Run \`bun run build\` first.`,
  );
  process.exit(2);
}

const pages = walk(DIST);
if (pages.length === 0) {
  console.error(`analytics:check — ./${DIST} contains no HTML. Refusing to pass.`);
  process.exit(2);
}

const untagged: string[] = [];
const direct: string[] = [];

for (const page of pages) {
  const html = readFileSync(page, "utf8");
  const route = "/" + relative(DIST, page).replace(/\.html$/, "");
  if (!html.includes(EXPECTED)) untagged.push(route);
  if (html.includes(DIRECT)) direct.push(route);
}

if (untagged.length) {
  console.error(
    `analytics:check FAIL — ${untagged.length} page(s) carry no first-party Google tag:`,
  );
  for (const route of untagged) console.error(`  ${route}`);
}

if (direct.length) {
  console.error(
    `analytics:check FAIL — ${direct.length} page(s) reference ${DIRECT} directly, defeating the first-party path:`,
  );
  for (const route of direct) console.error(`  ${route}`);
}

if (untagged.length || direct.length) process.exit(1);

console.log(
  `analytics:check PASS — all ${pages.length} pages load the container from /mtag/gtm.js.`,
);
