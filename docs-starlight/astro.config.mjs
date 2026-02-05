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
    autogenerate: { directory: "03-features", collapsed: true },
    collapsed: true,
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
      { label: "Experiments", slug: "reference/experiments" },
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
    autogenerate: { directory: "07-process", collapsed: true },
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
        // TODO: update this once the docs live in `docs`.
        baseUrl:
          "https://github.com/gruntwork-io/terragrunt/edit/main/docs-starlight",
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
  redirects: {
    // Catch-all redirect from /docs/* to /*
    "/docs/[...slug]": "/[...slug]",

    // Root redirects
    "/": "/getting-started/quick-start/",
    "/docs/": "/getting-started/quick-start/",

    // Pages that have been rehomed.
    "/features/debugging/": "/troubleshooting/debugging/",
    "/upgrade/upgrading_to_terragrunt_0.19.x/": "/migrate/upgrading_to_terragrunt_0.19.x/",

    // Redirects to external sites.
    "/lp/[...slug]": "https://terragrunt.com/lp/[...slug]",
    "/terragrunt-ambassador": "https://terragrunt.com/terragrunt-ambassador",
    "/terragrunt-ambassador/[...slug]": "https://terragrunt.com/terragrunt-ambassador/[...slug]",
    "/terragrunt-scale": "https://terragrunt.com/terragrunt-ambassador",
    "/terragrunt-scale/[...slug]": "https://terragrunt.com/terragrunt-ambassador/[...slug]",
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
    "/features/aws-authentication/": "/features/authentication/",
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
    "/features/keep-your-terragrunt-architecture-dry/": "/features/includes/",
    "/features/keep-your-remote-state-configuration-dry/": "/features/state-backend/",
    "/features/keep-your-cli-flags-dry/": "/features/extra-arguments/",
    "/features/aws-auth/": "/features/aws-authentication/",
    "/features/work-with-multiple-aws-accounts/": "/features/aws-authentication/",
    "/features/auto-retry/": "/features/runtime-control/",
    "/features/provider-cache/": "/features/provider-cache-server/",
    "/features/provider-caching/": "/features/provider-cache-server/",

    // Additional redirects for 404ing URLs
    "/features/execute-terraform-commands-on-multiple-modules-at-once/": "/features/stacks/",
    "/getting-started/configuration/": "/reference/hcl/",
    "/features/before-and-after-hooks/": "/features/hooks/",
    "/etting-started/configuration/": "/reference/hcl/", // typo in original URL
    "/features/log-formatting": "/reference/logging/formatting/",
    "/reference/lock-file-handling/": "/reference/lock-files/",

    // Restructured docs
    "/reference/cli/rules": "/process/cli-rules/",

    // Redirects for external resources
    "/community/invite": "https://discord.com/invite/YENaT9h8jh",
  },
  vite: {
    plugins: [tailwindcss()],
  },
});
