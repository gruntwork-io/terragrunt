#!/usr/bin/env bun
// Ping IndexNow with every URL in the sitemap at the end of a production build.
// No-op outside Vercel production. Never throws — IndexNow is best-effort.

import { join } from "node:path";

const KEY = "7a409eaf64d4ae9f009a70196fd234cd";
const PROD_HOST = "docs.terragrunt.com";
const KEY_LOCATION = `https://${PROD_HOST}/${KEY}.txt`;
const ENDPOINT = "https://api.indexnow.org/indexnow";
const BATCH_SIZE = 10000;

const log = (msg) => console.log(`[indexnow] ${msg}`);

async function main() {
  if (!isProductionBuild()) return;

  const sitemapFiles = await findSitemapFiles();
  if (sitemapFiles.length === 0) {
    log("no sitemap files found, skipping");
    return;
  }
  log(`found ${sitemapFiles.length} sitemap file(s): ${sitemapFiles.join(", ")}`);

  const urls = (await Promise.all(sitemapFiles.map(extractUrlsFromSitemap))).flat();
  if (urls.length === 0) {
    log("no production URLs extracted, skipping");
    return;
  }

  await submitBatches(urls);
}

function isProductionBuild() {
  const env = process.env.VERCEL_ENV ?? "unset";
  if (env === "production") return true;
  log(`skipping (VERCEL_ENV=${env})`);
  return false;
}

async function findSitemapFiles() {
  // Astro Vercel adapter emits to .vercel/output/static; local `output: "server"`
  // build (Node adapter) emits to dist/client.
  const candidates = [".vercel/output/static", "dist/client"];
  const glob = new Bun.Glob("sitemap-*.xml");
  for (const dir of candidates) {
    const matches = [];
    try {
      for await (const f of glob.scan({ cwd: dir })) {
        // Glob matches sitemap-index.xml too; we only want the numbered shards.
        if (/^sitemap-\d+\.xml$/.test(f)) matches.push(join(dir, f));
      }
    } catch {
      // Directory doesn't exist or isn't readable — try next candidate.
    }
    if (matches.length > 0) return matches;
  }
  return [];
}

async function extractUrlsFromSitemap(file) {
  const xml = await Bun.file(file).text();
  const urls = [];
  for (const match of xml.matchAll(/<loc>([^<]+)<\/loc>/g)) {
    const url = match[1].trim();
    // sitemap-index.xml contains <loc> entries pointing at sub-sitemaps —
    // skip those, we already glob them directly.
    if (url.endsWith(".xml")) continue;
    if (hostOf(url) === PROD_HOST) urls.push(url);
  }
  return urls;
}

function hostOf(url) {
  try {
    return new URL(url).host;
  } catch {
    return null;
  }
}

async function submitBatches(urls) {
  log(`submitting ${urls.length} URLs in batches of ${BATCH_SIZE}`);
  for (let i = 0; i < urls.length; i += BATCH_SIZE) {
    const batchNumber = i / BATCH_SIZE + 1;
    await submitBatch(urls.slice(i, i + BATCH_SIZE), batchNumber);
  }
}

async function submitBatch(batch, batchNumber) {
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
    if (res.ok) {
      log(`batch ${batchNumber} (${batch.length} URLs): HTTP ${res.status}`);
    } else {
      const snippet = (await res.text()).trim().slice(0, 500);
      log(`batch ${batchNumber} (${batch.length} URLs) failed: HTTP ${res.status} response: "${snippet}"`);
    }
  } catch (err) {
    log(`batch ${batchNumber} failed: ${err?.message ?? err}`);
  }
}

main().catch((err) => {
  log(`unexpected error (ignored): ${err?.message ?? err}`);
});
