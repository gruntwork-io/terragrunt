/**
 * Shared chrome content. Copy lives here rather than inside the components so a
 * non-engineer can change a nav label or a footer link without reading markup.
 */

export const GITHUB_REPO = "https://github.com/gruntwork-io/terragrunt";
export const DISCORD_INVITE = "https://discord.com/invite/YENaT9h8jh";
export const DOCS_URL = "https://docs.terragrunt.com";
export const ENTERPRISE_SUPPORT_URL =
  "https://www.gruntwork.io/services/terragrunt";

/**
 * Shown next to the GitHub icon in the nav. The live site renders a number
 * baked in at publish time; this is the same idea, stated rather than fetched,
 * so a build never depends on GitHub's API being reachable.
 */
export const GITHUB_STARS = "9.7k";

export const navLinks = [
  { label: "Docs", href: DOCS_URL },
  { label: "Ambassadors", href: "/terragrunt-ambassador" },
] as const;

export const footerLinks = {
  left: [
    { label: "AI & LLM Info", href: "/ai-info-page" },
    { label: "FAQs", href: "/faqs-overview" },
  ],
  right: [
    { label: "TGS Comparisons", href: "/terragrunt-scale-comparisons" },
    { label: "TGS Guides", href: "/terragrunt-scale-guides" },
  ],
} as const;

export const openSourceTools = [
  {
    name: "Terratest",
    description: "Validate IaC modules",
    href: "https://terratest.gruntwork.io/",
  },
  {
    name: "Boilerplate",
    description: "Automate repetitive DevOps work",
    href: "https://github.com/gruntwork-io/boilerplate",
  },
  {
    name: "CloudNuke",
    description: "Save money on unused AWS resources",
    href: "https://github.com/gruntwork-io/cloud-nuke",
  },
  {
    name: "GitXargs",
    description: "Make changes across git repos",
    href: "https://github.com/gruntwork-io/git-xargs",
  },
  {
    name: "Runbooks",
    description: "Scale your expertise",
    href: "https://github.com/gruntwork-io/runbooks",
  },
] as const;

/** The three Terragrunt Scale products, shown in the CTA band on most pages. */
export const scaleProducts = [
  {
    title: "Automate your CI/CD",
    product: "Terragrunt Pipelines",
    body: "is an out-of-the-box, best practices, secure CI/CD pipeline, built for Terragrunt, by the creators of Terragrunt.",
  },
  {
    title: "Remediate infra drift",
    product: "Terragrunt Drift Detection",
    body: "regularly compares your live cloud resources to your IaC and automatically opens pull requests to eliminate drift.",
  },
  {
    title: "Keep your IaC Fresh",
    product: "Terragrunt Patcher",
    body: "is a dependency management tool that keeps your OpenTofu/Terraform modules up to date, even through breaking changes.",
  },
] as const;
