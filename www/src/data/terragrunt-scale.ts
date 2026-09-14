/**
 * Terragrunt Scale page copy, lifted from the Webflow build.
 *
 * Pricing is data rather than markup on purpose: the plan names, unit ceilings
 * and the annual-discount footnote are the parts most likely to change, and a
 * price buried in a template is one nobody can find.
 */

export interface Plan {
  /** Shown above the plan name, e.g. "Free Forever" or "Starting At". */
  priceLabel: string;
  price: string;
  /** Marks the price with a dagger tied to `footnote`. */
  priceFootnote?: boolean;
  name: string;
  units: string;
  audience: string;
  /** Introduces the feature list, e.g. "Everything in Team, plus...". */
  upsell?: string;
  features: readonly string[];
  footnote?: string;
  cta: { label: string; href: string; variant: "primary" | "secondary" };
}

export const hero = {
  eyebrow: "Terragrunt Scale",
  headlineLead: "Get",
  headline:
    "production-grade CI/CD with plans in your pull requests, plus drift detection, and automatic updates, from the creators of Terragrunt.",
  /** Phrases rendered in the accent colour inside the headline. */
  accents: [
    "production-grade CI/CD",
    "drift detection",
    "automatic updates",
  ],
  builtForLabel: "Built For:",
  builtFor: ["Github", "Gitlab", "Terragrunt"],
  assurances: [
    "No credit card required",
    "No local CLI or auth to set up",
    "Be live with real infra in 10 minutes",
    "Runs in your CI",
  ],
  primary: {
    label: "Get Free CI/CD",
    href: "https://app.gruntwork.io/signup/terragrunt-scale-free-tier",
  },
  secondary: { label: "View Plans", href: "#view-plans" },
} as const;

export const video = {
  duration: "2 min",
  caption: "Watch a terragrunt plan run in response to a pull request",
  /**
   * Vimeo, embedded directly rather than through Webflow's embedly wrapper —
   * one fewer third-party host on the critical path, and embedly added nothing
   * but a redirect.
   */
  vimeoId: "1118246701",
} as const;

export const howItWorks = {
  eyebrow: "GitOps Workflow",
  headlineLead: "How",
  headlineAccent: "it works",
  steps: [
    {
      title: "1. Create a pull request",
      body: "Open a pull request and Terragrunt Scale automatically runs a plan. Smart change detection identifies the minimum set of infrastructure units affected by your changes to keep the blast radius small.",
    },
    {
      title: "2. Review the plan",
      body: "The Terragrunt plan is posted in the pull request comment, with resource summaries and full plan output for each affected infrastructure unit.",
    },
    {
      title: "3. Merge to apply",
      body: "Merge the pull request to apply. Changes roll out in dependency order across your units, stacks, and environments, so main always reflects what's deployed.",
    },
  ],
} as const;

export const pipelines = {
  eyebrow: "Pipelines",
  headlineLead: "Deploy",
  headlineAccent: "infra changes with confidence",
  image: "tgs-pipelines-plan.png",
  imageAlt: "Terragrunt plan shown in a pull request comment",
  caption: "Per-unit Terragrunt plan output is shown in a pull request comment.",
  features: [
    {
      title: "Secure GitOps Workflow",
      body: "Plans run automatically when you open a pull request, and applies run on merge in your GitHub/GitLab workflows, not on our servers. Plan output and log summaries appear in comments for easy access.",
    },
    {
      title: "Smart Dependency Management",
      body: "Updates are applied while respecting the dependency graph (DAG) order so that adds, changes, and destroys all run successfully, even across environments with their own access control rules.",
    },
    {
      title: "Minimize Your Blast Radius",
      body: "Pipelines identifies the minimum set of infrastructure units affected by your changes, avoiding unnecessary operations and limiting risk.",
    },
  ],
} as const;

export const driftDetection = {
  eyebrow: "Drift Detection",
  headlineLead: "Detect and resolve",
  headlineAccent: "drift",
  image: "tgs-drift-detected.png",
  imageAlt: "A drift remediation pull request opened by Terragrunt Scale",
  caption: "Drift remediation pull requests are opened automatically.",
  features: [
    {
      title: "Scheduled Runs",
      body: "Run drift detection as often as you like to ensure your live resources reflect your IaC.",
    },
    {
      title: "Automatic Pull Requests",
      body: "Get pull requests to automatically report and resolve drift. Nothing changes in your infrastructure without review and approval.",
    },
    {
      title: "Detect Drifted Stacks",
      body: "Detect drift even against your `terragrunt.stack.hcl` files to ensure nothing slips through the cracks.",
    },
  ],
} as const;

export const patcher = {
  eyebrow: "Patcher",
  headlineLead: "Update",
  headlineAccent: "stacks, units, and modules automatically",
  features: [
    {
      title: "Automatic Pull Requests",
      body: "Get PRs for dependency updates on your chosen schedule, customized to include one or many dependency changes.",
    },
    {
      title: "Promotion Workflows",
      body: "Patcher is aware of your environments, and can sequentially promote your updates across them.",
    },
    {
      title: "Update Terragrunt Stacks",
      body: "Detect updates even for units or nested stacks within your `terragrunt.stack.hcl` files.",
    },
  ],
} as const;

export const pricing = {
  headlineAccent: "Terragrunt Scale",
  headline: "Pricing",
  subhead: "All plans include unlimited runs and resources.",
  subheadEmphasis: "unlimited",
  unitFootnote: {
    before: "* An ",
    linkLabel: "infrastructure unit",
    href: "https://docs.terragrunt.com/getting-started/terminology/#unit",
    middle: " is any directory containing a ",
    code: "terragrunt.hcl",
    after: " file",
  },
  plans: [
    {
      priceLabel: "Free Forever",
      price: "$0",
      name: "Free",
      units: "Up to 25 infrastructure units*",
      audience:
        "For individuals, start-ups, or teams looking to try before buying",
      features: [
        "Terragrunt-native IaC Pipeline",
        "Unlimited Runs",
        "Unlimited Resources (no RUM)",
        "Community Support",
        "Get Started in 10 Minutes or Less",
        "No Credit Card Required",
      ],
      cta: {
        label: "Get Started",
        href: "https://app.gruntwork.io/signup/terragrunt-scale-free-tier",
        variant: "primary",
      },
    },
    {
      priceLabel: "Starting At",
      price: "$500/mo",
      priceFootnote: true,
      name: "Team",
      units: "200 infrastructure units*",
      audience: "For growing platform teams in production",
      features: [
        "Terragrunt-native IaC Pipeline",
        "Terragrunt-native Automatic Updates",
        "Terragrunt-native Drift Detection",
        "Unlimited Runs",
        "Unlimited Resources (no RUM)",
        "Standard Support",
      ],
      footnote: "Price reflects a 17% annual contract discount",
      cta: {
        label: "Buy Now",
        href: "/terragrunt-scale/checkout",
        variant: "secondary",
      },
    },
    {
      priceLabel: "Custom Pricing",
      price: "",
      name: "Enterprise",
      units: "Unlimited infrastructure units*",
      audience:
        "For enterprises seeking scale, advanced functionality, and enterprise support",
      upsell: "Everything in Team, plus...",
      features: [
        "Self-hosted GitHub/GitLab Enterprise",
        "Air-gapped Environments",
        "FedRAMP & PCI Environments",
        "Premium Support",
      ],
      cta: {
        label: "Contact Sales",
        href: "/contact-tgs",
        variant: "secondary",
      },
    },
  ] satisfies Plan[],
};

export const testimonial = {
  quote:
    "Terragrunt makes managing our infrastructure across providers and environments consistent, safe, and easy to understand. And now with Pipelines, things are even easier… We can literally copy and paste infrastructure and spin up new services, providers, and even full environments in minutes instead of hours or days.",
  name: "Dallas Slaughter",
  role: "Founding Engineer",
} as const;

export const faqTeaser = {
  eyebrow: "FAQ",
  headlineAccent: "Frequently",
  headline: "Asked Questions",
  body: "Still have questions? Check out the FAQ section for answers or contact us directly.",
  links: {
    faq: "/faqs-overview",
    contact: "/contact-tgs",
  },
} as const;
