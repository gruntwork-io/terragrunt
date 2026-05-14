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
      {
        label: "Continuous Integration with Terragrunt",
        autogenerate: { directory: "02-guides/02-continuous-integration-with-terragrunt", collapsed: true },
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
      {
        label: "Strict Controls",
        items: [
          { label: "Overview", slug: "reference/strict-controls" },
          { label: "Active Controls", link: "/reference/strict-controls/active" },
          { label: "Completed Controls", link: "/reference/strict-controls/completed" },
        ],
      },
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
  {
    label: "Terragrunt Scale",
    autogenerate: { directory: "09-terragrunt-scale", collapsed: true },
    collapsed: true,
  },
];
