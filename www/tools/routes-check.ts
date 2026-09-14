/**
 * Assert the build still serves every URL Webflow published.
 *
 * This walks `dist/` rather than the page sources, because what a manifest
 * records is what was intended and what the build emits is what visitors get —
 * a route can be lost to a renamed collection field or a `getStaticPaths` that
 * silently returns fewer entries, neither of which changes a filename.
 *
 * WITH NO BUILD THE WALK RETURNS NOTHING, and an empty set of built routes
 * would make every manifest route look missing. That is reported as "no build"
 * rather than 55 failures, because a check whose ground truth is absent has to
 * say so instead of reasoning from nothing.
 */
import { readdirSync, existsSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

const DIST = "dist";
const MANIFEST = "manifest/routes.json";

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
    `routes:check — no build at ./${DIST}. Run \`bun run build\` first; this gate reads the build, not the sources.`,
  );
  process.exit(2);
}

const built = new Set(
  walk(DIST).map((file) => {
    const route = "/" + relative(DIST, file).replace(/\.html$/, "");
    return route === "/index" ? "/" : route;
  }),
);

if (built.size === 0) {
  console.error(`routes:check — ./${DIST} contains no HTML. Refusing to pass.`);
  process.exit(2);
}

const manifest = JSON.parse(readFileSync(MANIFEST, "utf8")) as {
  routes: string[];
};

const missing = manifest.routes.filter((route) => !built.has(route));
const extra = [...built].filter(
  (route) => !manifest.routes.includes(route) && route !== "/404",
);

if (missing.length) {
  console.error(
    `routes:check FAIL — ${missing.length} published URL(s) are no longer served:`,
  );
  for (const route of missing) console.error(`  ${route}`);
}

if (extra.length) {
  // Not a failure: new pages are expected. Printed so the manifest gets updated
  // deliberately rather than drifting until it means nothing.
  console.log(
    `routes:check — ${extra.length} route(s) built that predate no Webflow URL (add to the manifest if they are permanent):`,
  );
  for (const route of extra) console.log(`  ${route}`);
}

if (missing.length) process.exit(1);
console.log(
  `routes:check PASS — all ${manifest.routes.length} published URLs are served.`,
);
