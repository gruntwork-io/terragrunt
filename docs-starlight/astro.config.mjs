// @ts-check
import { defineConfig } from "astro/config";

import starlight from "@astrojs/starlight";
import sitemap from "@astrojs/sitemap";
import vercel from "@astrojs/vercel";
import partytown from "@astrojs/partytown";
import tailwindcss from "@tailwindcss/vite";
import react from "@astrojs/react";

import starlightLinksValidator from "starlight-links-validator";
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
          { label: "Overview", slug: "docs/reference/cli" },
          {
            label: "Commands",
            autogenerate: {
              directory: "04-reference/02-cli/02-commands",
              collapsed: true,
            },
          },
          { label: "Global Flags", slug: "docs/reference/cli/global-flags" },
          { label: "Rules", slug: "docs/reference/cli/rules" },
        ],
      },
      { label: "Strict Controls", slug: "docs/reference/strict-controls" },
      { label: "Experiments", slug: "docs/reference/experiments" },
      {
        label: "Supported Versions",
        slug: "docs/reference/supported-versions",
      },
      { label: "Lock Files", slug: "docs/reference/lock-files" },
      {
        label: "Logging",
        autogenerate: { directory: "04-reference/07-logging", collapsed: true },
      },
      { label: "Terragrunt Cache", slug: "docs/reference/terragrunt-cache" },
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
    label: "Migrate",
    autogenerate: { directory: "07-migrate", collapsed: true },
    collapsed: true,
  },
];

// https://astro.build/config
export default defineConfig({
  site: "https://terragrunt.gruntwork.io",
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
            content: 'https://terragrunt.gruntwork.io',
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
          href: "https://discord.gg/SPu4Degs5f",
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
            "/docs/reference/cli/commands/run#*",
            "/docs/reference/cli/commands/run/#*",
            "/docs/reference/cli/commands/list#*",
            "/docs/reference/cli/commands/list/#*",
          ],
        }),
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
    // Pages that have been rehomed.
    "/docs/features/debugging/": "/docs/troubleshooting/debugging/",
    "/docs/upgrade/upgrading_to_terragrunt_0.19.x/": "/docs/migrate/upgrading_to_terragrunt_0.19.x/",

    // Redirects to external sites.
    "/contact/": "https://gruntwork.io/contact",
    "/commercial-support/": "https://gruntwork.io/support",
    "/cookie-policy/": "https://gruntwork.io/legal/cookie-policy/",

    // Restructured docs
    "/docs/reference/configuration/": "/docs/reference/hcl/",
    "/docs/reference/cli-options/": "/docs/reference/cli/",
    "/docs/reference/built-in-functions/": "/docs/reference/hcl/functions/",
    "/docs/reference/config-blocks-and-attributes/": "/docs/reference/hcl/blocks/",
    "/docs/reference/strict-mode/": "/docs/reference/strict-controls/",
    "/docs/reference/log-formatting/": "/docs/reference/logging/formatting/",
    "/docs/features/aws-authentication/": "/docs/features/authentication/",
    "/docs/reference/experiment-mode/": "/docs/reference/experiments/",

    // Support old doc structure paths
    "/docs/": "/docs/getting-started/quick-start/",
    "/docs/getting-started/": "/docs/getting-started/quick-start/",
    "/docs/features/": "/docs/features/units/",
    "/docs/reference/": "/docs/reference/hcl/",
    "/docs/troubleshooting/": "/docs/troubleshooting/debugging/",
    "/docs/migrate/": "/docs/migrate/migrating-from-root-terragrunt-hcl/",

    // Support old community paths
    "/docs/community/": "/docs/community/contributing/",
    "/support/": "/docs/community/support/",

    // Support old feature paths
    "/docs/features/inputs/": "/docs/features/units/",
    "/docs/features/locals/": "/docs/features/units/",
    "/docs/features/keep-your-terraform-code-dry/": "/docs/features/units/",
    "/docs/features/execute-terraform-commands-on-multiple-units-at-once/": "/docs/features/stacks/",
    "/docs/features/keep-your-terragrunt-architecture-dry/": "/docs/features/includes/",
    "/docs/features/keep-your-remote-state-configuration-dry/": "/docs/features/state-backend/",
    "/docs/features/keep-your-cli-flags-dry/": "/docs/features/extra-arguments/",
    "/docs/features/aws-auth/": "/docs/features/aws-authentication/",
    "/docs/features/work-with-multiple-aws-accounts/": "/docs/features/aws-authentication/",
    "/docs/features/auto-retry/": "/docs/features/runtime-control/",
    "/docs/features/provider-cache/": "/docs/features/provider-cache-server/",
    "/docs/features/provider-caching/": "/docs/features/provider-cache-server/",

    // Additional redirects for 404ing URLs
    "/docs/features/execute-terraform-commands-on-multiple-modules-at-once/": "/docs/features/stacks/",
    "/docs/getting-started/configuration/": "/docs/reference/hcl/",
    "/docs/features/before-and-after-hooks/": "/docs/features/hooks/",
    "/docs/etting-started/configuration/": "/docs/reference/hcl/", // typo in original URL
    "/docs/features/log-formatting": "/docs/reference/logging/formatting/",
    "/docs/reference/lock-file-handling/": "/docs/reference/lock-files/",
  },
  vite: {
    plugins: [tailwindcss()],
  },
  tailwind: {
    // We include this extra Tailwind config to support the shadcn/ui components.
    configFile: './tailwind.config.mjs',
  },
});
