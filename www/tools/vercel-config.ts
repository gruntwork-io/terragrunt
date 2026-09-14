/**
 * Assert the invariants in `vercel.json` that nothing else can catch.
 *
 * Every rule here guards a failure that is silent in production and invisible
 * in a pull request. None of them is a style preference.
 */
import { readFileSync } from "node:fs";

interface Rule {
  source: string;
  destination: string;
}

interface VercelConfig {
  trailingSlash?: boolean;
  cleanUrls?: boolean;
  rewrites?: Rule[];
  redirects?: (Rule & { permanent?: boolean })[];
}

const config = JSON.parse(readFileSync("vercel.json", "utf8")) as VercelConfig;
const manifestRoutes = (
  JSON.parse(readFileSync("manifest/routes.json", "utf8")) as { routes: string[] }
).routes;
const failures: string[] = [];

const rewrites = config.rewrites ?? [];
const sources = rewrites.map((r) => r.source);

/*
 * VERCEL TAKES THE FIRST MATCHING REWRITE. `/mtag/:path*` matches
 * `/mtag/gtm.js`, so if the catch-all is listed first the container is served
 * as a plain proxy — Google's copy, with `www.googletagmanager.com` still baked
 * in — and the GA4 library is then fetched straight from Google, where a
 * blocker stops it. Analytics degrades silently and no page looks different.
 */
const functionRule = sources.indexOf("/mtag/gtm.js");
const proxyRule = sources.indexOf("/mtag/:path*");

if (functionRule === -1) {
  failures.push("no rewrite for /mtag/gtm.js — the GTM container is not served by api/mtag.js");
} else if (proxyRule === -1) {
  failures.push("no rewrite for /mtag/:path* — the container's own requests will 404");
} else if (functionRule > proxyRule) {
  failures.push(
    `/mtag/gtm.js (index ${functionRule}) is listed AFTER /mtag/:path* (index ${proxyRule}). ` +
      "Vercel takes the first match, so the catch-all shadows the function and GA4 loads from Google.",
  );
}

/* The form endpoint is the lead path. Without it every submission 404s. */
if (!sources.includes("/ftag")) {
  failures.push("no rewrite for /ftag — form submissions reach nothing");
}

/*
 * Webflow served every URL without a trailing slash and 301'd the slashed form.
 * Flipping this turns every inbound link into a redirect hop and splits link
 * equity across two spellings of every URL.
 */
if (config.trailingSlash !== false) {
  failures.push(
    `trailingSlash is ${JSON.stringify(config.trailingSlash)}; must be false to match the URLs already indexed`,
  );
}

/*
 * `cleanUrls` must be FALSE explicitly, not merely omitted. Enabling it injects
 * a 308 matching `^/(.*)\.html/?$` AHEAD of the redirects array, which shadows
 * rules below it.
 */
if (config.cleanUrls !== false) {
  failures.push(
    `cleanUrls is ${JSON.stringify(config.cleanUrls)}; must be explicitly false`,
  );
}

/*
 * A redirect that points at itself is an infinite loop the browser reports as
 * ERR_TOO_MANY_REDIRECTS, and a duplicate source is a rule that silently never
 * fires because Vercel takes the first match. Both are trivial to introduce
 * when editing this file and neither is visible in a diff.
 */
const seen = new Set<string>();
for (const rule of config.redirects ?? []) {
  if (!rule.source.startsWith("/")) {
    failures.push(`redirect source "${rule.source}" must start with "/"`);
  }
  if (rule.source === rule.destination) {
    failures.push(`redirect "${rule.source}" points at itself`);
  }
  if (seen.has(rule.source)) {
    failures.push(
      `redirect source "${rule.source}" is listed twice; Vercel takes the first match, so the later rule never fires`,
    );
  }
  seen.add(rule.source);
}

/*
 * A redirect whose source is also served as a page never fires — the static
 * file wins — so a rule added to move a URL would appear to do nothing.
 */
for (const rule of config.redirects ?? []) {
  if (!rule.source.includes(":") && manifestRoutes.includes(rule.source)) {
    failures.push(
      `redirect source "${rule.source}" is also a built page; the page wins and the redirect is dead`,
    );
  }
}

if (failures.length) {
  console.error("vercel:check FAIL");
  for (const failure of failures) console.error(`  - ${failure}`);
  process.exit(1);
}

console.log(
  `vercel:check PASS — ${rewrites.length} rewrites, ${(config.redirects ?? []).length} redirects.`,
);
