import type { StarlightUserConfig } from "@astrojs/starlight/types";

export const sidebar: StarlightUserConfig["sidebar"] = [
  {
    label: "Getting Started",
    items: [{ autogenerate: { directory: "01-getting-started" } }],
  },
  {
    label: "Guides",
    items: [
      {
        label: "Terralith to Terragrunt",
        collapsed: true,
        items: [{ autogenerate: { directory: "02-guides/01-terralith-to-terragrunt", collapsed: true } }],
      },
      {
        label: "Continuous Integration with Terragrunt",
        collapsed: true,
        items: [{ autogenerate: { directory: "02-guides/02-continuous-integration-with-terragrunt", collapsed: true } }],
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
        items: [{ autogenerate: { directory: "03-features/01-units", collapsed: true } }],
      },
      {
        label: "Stacks",
        collapsed: true,
        items: [{ autogenerate: { directory: "03-features/02-stacks", collapsed: true } }],
      },
      {
        label: "Catalog",
        collapsed: true,
        items: [{ autogenerate: { directory: "03-features/06-catalog", collapsed: true } }],
      },
      {
        label: "Caching",
        collapsed: true,
        items: [
          { slug: "features/caching" },
          { slug: "features/caching/provider-cache-server" },
          { slug: "features/caching/auto-provider-cache-dir" },
          {
            label: "Content Addressable Store (CAS)",
            collapsed: true,
            items: [{ autogenerate: { directory: "03-features/07-caching/04-cas", collapsed: true } }],
          },
        ],
      },
      {
        label: "Filters",
        collapsed: true,
        items: [{ autogenerate: { directory: "03-features/08-filter", collapsed: true } }],
      },
    ],
  },
  {
    label: "Reference",
    collapsed: true,
    items: [
      {
        label: "HCL",
        collapsed: true,
        items: [{ autogenerate: { directory: "04-reference/01-hcl", collapsed: true } }],
      },
      {
        label: "CLI",
        collapsed: true,
        items: [
          { label: "Overview", slug: "reference/cli" },
          {
            label: "Commands",
            collapsed: true,
            items: [{ autogenerate: { directory: "04-reference/02-cli/02-commands", collapsed: true } }],
          },
          { label: "Global Flags", slug: "reference/cli/global-flags" },
        ],
      },
      {
        label: "Strict Controls",
        collapsed: true,
        items: [
          { label: "Overview", slug: "reference/strict-controls" },
          { label: "Active Controls", link: "/reference/strict-controls/active" },
          { label: "Completed Controls", link: "/reference/strict-controls/completed" },
        ],
      },
      {
        label: "Experiments",
        collapsed: true,
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
        collapsed: true,
        items: [{ autogenerate: { directory: "04-reference/07-logging", collapsed: true } }],
      },
      { label: "Terragrunt Cache", slug: "reference/terragrunt-cache" },
    ],
  },
  {
    label: "Community",
    collapsed: true,
    items: [{ autogenerate: { directory: "05-community", collapsed: true } }],
  },
  {
    label: "Troubleshooting",
    collapsed: true,
    items: [{ autogenerate: { directory: "06-troubleshooting", collapsed: true } }],
  },
  {
    label: "Process",
    collapsed: true,
    items: [
      { label: "Terragrunt 1.0 Guarantees", slug: "process/1-0-guarantees" },
      { label: "CLI Rules", slug: "process/cli-rules" },
      { label: "Releases", slug: "process/releases" },
      { label: "Changelog", link: "/process/changelog" },
    ],
  },
  {
    label: "Migrate",
    collapsed: true,
    items: [{ autogenerate: { directory: "08-migrate", collapsed: true } }],
  },
  {
    label: "Terragrunt Scale",
    collapsed: true,
    items: [{ autogenerate: { directory: "09-terragrunt-scale", collapsed: true } }],
  },
];
