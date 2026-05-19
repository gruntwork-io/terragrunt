#!/usr/bin/env bun
// Ping IndexNow with every URL in the sitemap at the end of a production build.
// No-op outside Vercel production. Never throws — IndexNow is best-effort.

import { readFileSync, existsSync, readdirSync } from "node:fs";
import { join } from "node:path";

const KEY = "7a409eaf64d4ae9f009a70196fd234cd";
const PROD_HOST = "docs.terragrunt.com";
const KEY_LOCATION = `https://${PROD_HOST}/${KEY}.txt`;
const ENDPOINT = "https://api.indexnow.org/indexnow";
const BATCH_SIZE = 10000;

const log = (msg) => console.log(`[indexnow] ${msg}`);

async function main() {
  const env = process.env.VERCEL_ENV ?? "unset";
  if (env !== "production") {
    log(`skipping (VERCEL_ENV=${env})`);
    return;
  }

  const sitemapFiles = findSitemapFiles();
  if (sitemapFiles.length === 0) {
    log("no sitemap files found, skipping");
    return;
  }
  log(`found ${sitemapFiles.length} sitemap file(s): ${sitemapFiles.join(", ")}`);

  const urls = [];
  for (const file of sitemapFiles) {
    const xml = readFileSync(file, "utf8");
    for (const match of xml.matchAll(/<loc>([^<]+)<\/loc>/g)) {
      const url = match[1].trim();
      // sitemap-index.xml contains <loc> entries pointing at sub-sitemaps —
      // skip those, we already glob them directly.
      if (url.endsWith(".xml")) continue;
      let host;
      try {
        host = new URL(url).host;
      } catch {
        continue;
      }
      if (host === PROD_HOST) urls.push(url);
    }
  }

  if (urls.length === 0) {
    log("no production URLs extracted, skipping");
    return;
  }
  log(`submitting ${urls.length} URLs in batches of ${BATCH_SIZE}`);

  for (let i = 0; i < urls.length; i += BATCH_SIZE) {
    const batch = urls.slice(i, i + BATCH_SIZE);
    const body = JSON.stringify({
      host: PROD_HOST,
      key: KEY,
      keyLocation: KEY_LOCATION,
      urlList: batch,
    });
    try {
      const res = await fetch(ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body,
      });
      log(`batch ${i / BATCH_SIZE + 1} (${batch.length} URLs): HTTP ${res.status}`);
    } catch (err) {
      log(`batch ${i / BATCH_SIZE + 1} failed: ${err?.message ?? err}`);
    }
  }
}

function findSitemapFiles() {
  // Astro Vercel adapter emits to .vercel/output/static; local `output: "server"`
  // build (Node adapter) emits to dist/client.
  const candidates = [".vercel/output/static", "dist/client"];
  for (const dir of candidates) {
    if (!existsSync(dir)) continue;
    const matches = readdirSync(dir)
      .filter((f) => /^sitemap-\d+\.xml$/.test(f))
      .map((f) => join(dir, f));
    if (matches.length > 0) return matches;
  }
  return [];
}

main().catch((err) => {
  log(`unexpected error (ignored): ${err?.message ?? err}`);
});
