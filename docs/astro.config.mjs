// @ts-check
import { defineConfig } from "astro/config";

import starlight from "@astrojs/starlight";
import sitemap from "@astrojs/sitemap";
import vercel from "@astrojs/vercel";
import partytown from "@astrojs/partytown";
import tailwindcss from "@tailwindcss/vite";
import react from "@astrojs/react";

import starlightLinksValidator from "starlight-links-validator";
import starlightLlmsTxt from "starlight-llms-txt";
import d2 from "astro-d2";

// Check if we're in Vercel environment
const isVercel = globalThis.process?.env?.VERCEL;

export const sidebar = [
  {
    label: "Getting Started",
    autogenerate: { directory: "01-getting-started" },
  },
  {
    label: "Guides",
    items: [
      {
        label: "Terralith to Terragrunt",
        autogenerate: { directory: "02-guides/01-terralith-to-terragrunt", collapsed: true },
      },
    ],
    collapsed: true,
  },
  {
    label: "Features",
    collapsed: true,
    items: [
      {
        label: "Units",
        collapsed: true,
        autogenerate: { directory: "03-features/01-units", collapsed: true },
      },
      {
        label: "Stacks",
        collapsed: true,
        autogenerate: { directory: "03-features/02-stacks", collapsed: true },
      },
      {
        label: "Catalog",
        collapsed: true,
        autogenerate: { directory: "03-features/06-catalog", collapsed: true },
      },
      {
        label: "Caching",
        collapsed: true,
        autogenerate: { directory: "03-features/07-caching", collapsed: true },
      },
      {
        label: "Filters",
        collapsed: true,
        autogenerate: { directory: "03-features/08-filter", collapsed: true },
      },
    ],
  },
  {
    label: "Reference",
    collapsed: true,
    items: [
      {
        label: "HCL",
        autogenerate: { directory: "04-reference/01-hcl", collapsed: true },
      },
      {
        label: "CLI",
        collapsed: true,
        items: [
          { label: "Overview", slug: "reference/cli" },
          {
            label: "Commands",
            autogenerate: {
              directory: "04-reference/02-cli/02-commands",
              collapsed: true,
            },
          },
          { label: "Global Flags", slug: "reference/cli/global-flags" },
        ],
      },
      { label: "Strict Controls", slug: "reference/strict-controls" },
      {
        label: "Experiments",
        items: [
          { label: "Overview", slug: "reference/experiments" },
          { label: "Active Experiments", link: "/reference/experiments/active" },
          { label: "Completed Experiments", link: "/reference/experiments/completed" },
        ],
      },
      {
        label: "Supported Versions",
        slug: "reference/supported-versions",
      },
      { label: "Lock Files", slug: "reference/lock-files" },
      {
        label: "Logging",
        autogenerate: { directory: "04-reference/07-logging", collapsed: true },
      },
      { label: "Terragrunt Cache", slug: "reference/terragrunt-cache" },
    ],
  },
  {
    label: "Community",
    autogenerate: { directory: "05-community", collapsed: true },
    collapsed: true,
  },
  {
    label: "Troubleshooting",
    autogenerate: { directory: "06-troubleshooting", collapsed: true },
    collapsed: true,
  },
  {
    label: "Process",
    items: [
      { label: "Terragrunt 1.0 Guarantees", slug: "process/1-0-guarantees" },
      { label: "CLI Rules", slug: "process/cli-rules" },
      { label: "Releases", slug: "process/releases" },
      { label: "Changelog", link: "/process/changelog" },
    ],
    collapsed: true,
  },
  {
    label: "Migrate",
    autogenerate: { directory: "08-migrate", collapsed: true },
    collapsed: true,
  },
];

// https://astro.build/config
export default defineConfig({
  site: "https://docs.terragrunt.com",
  base: "/",
  output: isVercel ? "server" : "static",
  adapter: isVercel
    ? vercel({
      imageService: true,
      isr: {
        expiration: 60 * 60 * 24, // 24 hours
      },
    })
    : undefined,
  integrations: [
    // We use React for the shadcn/ui components.
    react(),
    starlight({
      title: "Terragrunt",
      description: "Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.",
      editLink: {
        baseUrl:
          "https://github.com/gruntwork-io/terragrunt/edit/main/docs",
      },
      customCss: ["./src/styles/global.css"],
      head: [
        {
          tag: 'meta',
          attrs: {
            name: 'description',
            content: 'Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.',
          },
        },
        {
          tag: 'meta',
          attrs: {
            property: 'og:title',
            content: 'Terragrunt',
          },
        },
        {
          tag: 'meta',
          attrs: {
            property: 'og:description',
            content: 'Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.',
          },
        },
        {
          tag: 'meta',
          attrs: {
            property: 'og:type',
            content: 'website',
          },
        },
        {
          tag: 'meta',
          attrs: {
            property: 'og:url',
            content: 'https://docs.terragrunt.com',
          },
        },
        {
          tag: 'meta',
          attrs: {
            name: 'twitter:card',
            content: 'summary_large_image',
          },
        },
        {
          tag: 'meta',
          attrs: {
            name: 'twitter:title',
            content: 'Terragrunt',
          },
        },
        {
          tag: 'meta',
          attrs: {
            name: 'twitter:description',
            content: 'Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.',
          },
        },
      ],
      components: {
        Header: "./src/components/Header.astro",
        PageSidebar: "./src/components/PageSidebar.astro",
        SiteTitle: "./src/components/SiteTitle.astro",
        SkipLink: "./src/components/SkipLink.astro",
      },
      logo: {
        dark: "/src/assets/horizontal-logo-light.svg",
        light: "/src/assets/horizontal-logo-dark.svg",
      },
      social: [
        {
          href: "/community/invite",
          icon: "discord",
          label: "Discord",
        },
      ],
      sidebar: sidebar,
      plugins: [
        starlightLinksValidator({
          exclude: [
            // Used in the docs for OpenTelemetry
            "http://localhost:16686/",
            "http://localhost:9090/",

            // Unfortunately, these have to be ignored, as they're referencing content
            // that is generated outside the contents of the markdown file.
            "/reference/cli/commands/run#*",
            "/reference/cli/commands/run/#*",
            "/reference/cli/commands/list#*",
            "/reference/cli/commands/list/#*",
            "/reference/cli/commands/find#*",
            "/reference/cli/commands/find/#*",

            // Custom .astro pages for experiments — can't be validated statically
            "/reference/experiments/active#*",
            "/reference/experiments/completed#*",

            // Used as a redirect to the Terragrunt Discord server
            "/community/invite",
          ],
        }),
        starlightLlmsTxt()
      ],
    }),
    d2({
      // It's recommended that we just skip generation in Vercel,
      // and generate diagrams locally:
      // https://astro-d2.vercel.app/guides/how-astro-d2-works/#deployment
      skipGeneration: !!isVercel,
    }),
    partytown({
      config: {
        debug: false,
        logCalls: false,
        logGetters: false,
        logSetters: false,
        logImageRequests: false,
        logScriptExecution: false,
        logStackTraces: false,
        forward: ['dataLayer.push'],
      },
    }),
    sitemap(),
  ],
  // Note that some redirects are handled in vercel.json instead.
  //
  // This is because Astro won't do dynamic redirects for external destinations.
  // It's faster to have Vercel handle it anyways.
  redirects: {
    // Catch-all redirect from /docs/* to /*
    "/docs/[...slug]": "/[...slug]",

    // Root redirects
    "/": "/getting-started/quick-start/",
    "/docs/": "/getting-started/quick-start/",

    // Pages that have been rehomed.
    "/features/scaffold/": "/features/catalog/scaffold/",
    "/features/run-queue/": "/features/stacks/run-queue/",
    "/features/debugging/": "/troubleshooting/debugging/",
    "/upgrade/upgrading_to_terragrunt_0.19.x/": "/migrate/upgrading_to_terragrunt_0.19.x/",

    // Merged pages
    "/features/stacks/dependencies/": "/features/stacks/stack-operations/",
    "/features/stacks/orchestration/": "/features/stacks/stack-operations/",

    // Redirects to external sites.
    "/terragrunt-ambassador": "https://terragrunt.com/terragrunt-ambassador",
    "/terragrunt-scale": "https://terragrunt.com/terragrunt-scale",
    "/contact/": "https://gruntwork.io/contact",
    "/commercial-support/": "https://gruntwork.io/support",
    "/cookie-policy/": "https://gruntwork.io/legal/cookie-policy/",

    // Restructured docs
    "/reference/configuration/": "/reference/hcl/",
    "/reference/cli-options/": "/reference/cli/",
    "/reference/built-in-functions/": "/reference/hcl/functions/",
    "/reference/config-blocks-and-attributes/": "/reference/hcl/blocks/",
    "/reference/strict-mode/": "/reference/strict-controls/",
    "/reference/log-formatting/": "/reference/logging/formatting/",
    "/features/aws-authentication/": "/features/units/authentication/",
    "/reference/experiment-mode/": "/reference/experiments/",

    // Support old doc structure paths
    "/getting-started/": "/getting-started/quick-start/",
    "/features/": "/features/units/",
    "/reference/": "/reference/hcl/",
    "/troubleshooting/": "/troubleshooting/debugging/",
    "/migrate/": "/migrate/migrating-from-root-terragrunt-hcl/",

    // Support old community paths
    "/community/": "/community/contributing/",
    "/support/": "/community/support/",

    // Support old feature paths
    "/features/inputs/": "/features/units/",
    "/features/locals/": "/features/units/",
    "/features/keep-your-terraform-code-dry/": "/features/units/",
    "/features/execute-terraform-commands-on-multiple-units-at-once/": "/features/stacks/",
    "/features/keep-your-terragrunt-architecture-dry/": "/features/units/includes/",
    "/features/keep-your-remote-state-configuration-dry/": "/features/units/state-backend/",
    "/features/keep-your-cli-flags-dry/": "/features/units/extra-arguments/",
    "/features/aws-auth/": "/features/units/authentication/",
    "/features/work-with-multiple-aws-accounts/": "/features/units/authentication/",
    "/features/auto-retry/": "/features/units/runtime-control/",
    "/features/provider-cache/": "/features/caching/provider-cache-server/",
    "/features/provider-caching/": "/features/caching/provider-cache-server/",
    "/features/engine/": "/features/units/engine/",
    "/features/run-report/": "/features/stacks/run-report/",
    "/features/provider-cache-server/": "/features/caching/provider-cache-server/",
    "/features/auto-provider-cache-dir/": "/features/caching/auto-provider-cache-dir/",
    "/features/cas/": "/features/caching/cas/",

    // Additional redirects for 404ing URLs
    "/features/execute-terraform-commands-on-multiple-modules-at-once/": "/features/stacks/",
    "/getting-started/configuration/": "/reference/hcl/",
    "/features/before-and-after-hooks/": "/features/units/hooks/",
    "/etting-started/configuration/": "/reference/hcl/", // typo in original URL
    "/features/log-formatting": "/reference/logging/formatting/",
    "/reference/lock-file-handling/": "/reference/lock-files/",

    // Restructured docs
    "/reference/cli/rules": "/process/cli-rules/",

    // Unit features rehomed under /features/units/
    "/features/includes/": "/features/units/includes/",
    "/features/state-backend/": "/features/units/state-backend/",
    "/features/extra-arguments/": "/features/units/extra-arguments/",
    "/features/authentication/": "/features/units/authentication/",
    "/features/hooks/": "/features/units/hooks/",
    "/features/auto-init/": "/features/units/auto-init/",
    "/features/runtime-control/": "/features/units/runtime-control/",

    // Redirects for external resources
    "/community/invite": "https://discord.com/invite/YENaT9h8jh",
  },
  vite: {
    plugins: [
      tailwindcss(),
      {
        name: 'compatibility-query-redirect',
        configureServer(server) {
          server.middlewares.use((req, _res, next) => {
            const url = req.url ?? '';
            if (url === '/api/v1/compatibility' || url.startsWith('/api/v1/compatibility?')) {
              const qs = url.includes('?') ? url.split('?')[1] : '';
              const tool = new URLSearchParams(qs).get('tool');
              if (tool === 'opentofu' || tool === 'terraform') {
                req.url = `/api/v1/compatibility/${tool}`;
              } else {
                req.url = '/api/v1/compatibility/index';
              }
            }
            next();
          });
        },
      },
    ],
  },
});
