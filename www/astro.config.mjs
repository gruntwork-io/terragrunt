// @ts-check
import { defineConfig } from "astro/config";
import mdx from "@astrojs/mdx";
import sitemap from "@astrojs/sitemap";
import vercel from "@astrojs/vercel";
import tailwindcss from "@tailwindcss/vite";

const isVercel = !!globalThis.process?.env?.VERCEL;

// https://astro.build/config
export default defineConfig({
  site: "https://terragrunt.com",

  // Webflow served every page without a trailing slash and 301'd the slashed
  // form. Matching that is URL parity, not preference: a mismatch turns every
  // inbound link into a redirect hop and splits link equity across two spellings.
  trailingSlash: "never",
  build: { format: "file" },

  output: "static",
  adapter: isVercel ? vercel({ imageService: false }) : undefined,

  integrations: [mdx(), sitemap()],

  /**
   * Pin the CSS minifier's target. Left to its default it rewrites
   * `(max-width: 991px)` into range syntax that needs Chrome 104 / Firefox 102 /
   * Safari 16.4 — below which the query never matches and a phone is served the
   * desktop layout. Valid CSS, clean build, silent failure on old browsers.
   */
  vite: {
    plugins: [tailwindcss()],
    build: { cssTarget: ["chrome87", "firefox78", "safari14", "edge88"] },
  },

  devToolbar: { enabled: false },
});
