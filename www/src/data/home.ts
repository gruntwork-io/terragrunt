/**
 * Homepage copy, lifted from the Webflow build of terragrunt.com.
 *
 * Docs links were retargeted from the retired `terragrunt.gruntwork.io/docs`
 * host to their current `docs.terragrunt.com` paths — the old spellings still
 * resolve, but only through two redirects.
 */

export interface FeatureCard {
  title: string;
  /** Inline markdown: links and emphasis only. Rendered by `renderLede`. */
  body: string;
}

export interface Testimonial {
  quote: string;
  name: string;
  role: string;
}

export const hero = {
  headline: "The Open Source IaC Orchestrator Platform Teams Trust",
  subhead:
    "Organize and speed up your Infrastructure as Code, with the #1 IaC Orchestrator",
  builtForLabel: "Built For:",
  builtFor: ["OpenTofu", "Terraform"],
  primary: { label: "Free CI/CD", href: "/terragrunt-scale" },
  secondary: {
    label: "Quick Start",
    href: "https://docs.terragrunt.com/getting-started/quick-start/",
  },
} as const;

export const notificationBar = {
  text: "Free Terragrunt-native CI/CD pipeline. Unlimited runs and resources.",
  href: "/terragrunt-scale",
} as const;

export const featuredBrandsHeadline = "Managing Infrastructure At";

export const scalePanel = {
  headline: "The GitOps Pipeline Built for Terragrunt",
  builtForLabel: "Built For:",
  body: "Get a production-ready infrastructure pipeline that runs in your CI runners. Free for up to 25 infrastructure units.",
  cta: { label: "Get Free CI/CD for Terragrunt", href: "/terragrunt-scale" },
} as const;

export const orchestrateSection = {
  eyebrow: "Orchestrate",
  headline: "Orchestrate Your",
  headlineAccent: "Infrastructure",
  cards: [
  {
    "title": "Segment Your Infrastructure",
    "body": "Break down your monolithic infrastructure into manageable [units](https://docs.terragrunt.com/features/units/) that are safe to update independently and have their own state"
  },
  {
    "title": "Control Your Infrastructure",
    "body": "Reject all-or-nothing updates. Control exactly what infrastructure is updated, how it gets updated, and in what order by taking advantage of the [Run Queue](https://docs.terragrunt.com/features/stacks/run-queue/)."
  },
  {
    "title": "Secure Your Infrastructure",
    "body": "Terragrunt [authentication tooling](https://docs.terragrunt.com/features/units/authentication/) makes it trivial to manage infrastructure across any number of environments or cloud providers with least privilege access."
  }
] satisfies FeatureCard[],
};

export const consistencySection = {
  eyebrow: "Consistency",
  headline: ["Codify", "Tribal", "Knowledge"],
  secondHeadline: ["and", "Enable", "Self-Service"],
  /** The first three sit above the second headline; the last two below it. */
  cards: [
  {
    "title": "Automate Using Hooks",
    "body": "Don’t pretend you’ll always remember to do things before/after updating IaC. Codify those tasks with [hooks](https://docs.terragrunt.com/features/units/hooks/) so that they’re done automatically."
  },
  {
    "title": "Handle Expected Errors",
    "body": "You know networks can be flaky, and cloud providers aren’t perfect. Automatically handle the errors they produce using built-in [error handling](https://docs.terragrunt.com/features/units/runtime-control/#errors)."
  },
  {
    "title": "Integrate Early and Often",
    "body": "Get you and your teammates on the same page. Work on a consistent, unified codebase, but gradually roll out new infrastructure features using [feature flags](https://docs.terragrunt.com/features/units/runtime-control/#feature-flags)."
  },
  {
    "title": "Leverage a Proven Infrastructure Catalog",
    "body": "Catalog standard infrastructure patterns for reuse across your organization using the [catalog](https://docs.terragrunt.com/features/catalog/) Terminal User Interface (TUI)."
  },
  {
    "title": "Empower All Your Engineers",
    "body": "Design opinionated templates to [scaffold](https://docs.terragrunt.com/features/catalog/scaffold/) exactly the right infrastructure that’s needed without writing any IaC by hand."
  }
] satisfies FeatureCard[],
};

export const drySection = {
  eyebrow: "DRY",
  headlineAccent: "Reduce",
  headline: "Code Repetition",
  cards: [
  {
    "title": "Reuse Common Infrastructure Configurations",
    "body": "Define reusable infrastructure configurations, like module inputs, backend configurations, and providers once, then reference them wherever they’re needed using [includes](https://docs.terragrunt.com/features/units/includes/)."
  },
  {
    "title": "Reuse Production-Ready Patterns",
    "body": "Leverage [Terragrunt Stack](https://docs.terragrunt.com/features/stacks/) to encapsulate higher level collections of infrastructure components as a unified stack."
  }
] satisfies FeatureCard[],
};

export const testimonialsSection = {
  eyebrow: "Testimonials",
  headlineAccent: "Beloved",
  headline: "by the Community",
  items: [
  {
    "quote": "Using Terragrunt to organize my Terraform files using DRY standards. Highly recommend it if you're repeating your code in your project.",
    "name": "Peter Kay",
    "role": "SRE"
  },
  {
    "quote": "I use Terragrunt—and I never look back. Some say Terragrunt is overkill. Some say it's only for 'advanced use cases.' I say: if you're serious about Infrastructure as Code, it's a must-have.",
    "name": "Sergei Li",
    "role": "Staff Engineer"
  },
  {
    "quote": "If you're managing multiple Terraform environments and things feel brittle or duplicated—Terragrunt is not just 'nice to have', it's a game-changer.",
    "name": "Roman Bessembe",
    "role": "Senior DevOps Engineer"
  },
  {
    "quote": "When it comes to managing and deploying multiple copies of the same infrastructure to the same or different AWS account, an additional tool is needed. And that tool has the name Terragrunt.",
    "name": "Naveen Pantera",
    "role": "Principal Engineer"
  },
  {
    "quote": "Terragrunt is a game-changer for handling state files and modules!",
    "name": "Victor Garcia",
    "role": "Cloud DevOps Engineer"
  },
  {
    "quote": "Terragrunt shines with its 'before' and 'after' hooks, allowing you to automate repetitive tasks such as logging, monitoring setup, or even policy enforcement checks before applying changes.",
    "name": "Samay Singh Bisht",
    "role": "DevOps Engineer"
  },
  {
    "quote": "Terragrunt is really a cutting edge IaC technology.",
    "name": "Chidubem Chinwuba",
    "role": "DevOps Engineer"
  },
  {
    "quote": "If you are using Terraform to setup your infrastructure and you're not also using Terragrunt you should take a look at it.",
    "name": "Darryl Ruggles",
    "role": "Cloud Solutions Architect"
  },
  {
    "quote": "Terragrunt is actually sweet, makes environment handling easy, DRY- Don't repeat yourself.",
    "name": "Dimeji Ojewunmi",
    "role": "Cloud DevOps Engineer"
  },
  {
    "quote": "I've been using Terragrunt for a year now as the only SRE in early stages startup that required to build everything from Scratch and It truly gave me peace of mind",
    "name": "Anastasia Kondratieva",
    "role": "SRE"
  },
  {
    "quote": "Personally, I can't imagine a terraform setup without terragrunt anymore. I love that I can have common inputs defined in one place, with ability to generate resource name prefixes and value overrides (e.g. for subscription IDs) based on a stack's file path. With that, very few inputs need to be passed explicitly, which makes it super-easy to scale and to make changes across stacks.",
    "name": "Igor Beliakov",
    "role": "Senior Platform Engineer"
  },
  {
    "quote": "Terragrunt helps manage those pesky repetitive tasks you don't even realize are sucking up your time. Think of it as Terraform's cooler, more organized sibling. I spent an afternoon cleaning up a setup where each team had cooked up their own deployment scripts. A total mess! Terragrunt's module inheritance saved the day - one config to rule them all.",
    "name": "Petar Nikov",
    "role": "DevOps Engineer"
  },
  {
    "quote": "It's safe to say it's earned a permanent spot in my DevOps toolkit. While Terraform is already a powerful tool for defining infrastructure as code, Terragrunt builds on it - especially when you're managing multiple environments like dev, staging, and prod.",
    "name": "Gerald Akenji",
    "role": "DevOps Cloud Engineer"
  },
  {
    "quote": "We've found that Terragrunt not only shortens our codebase by a few thousand lines, it also greatly simplifies our infrastructure code. We no longer need to repeat our provider, state, or dependency configurations, so the logic of our code is now focused exclusively on what it should be focusing on: the terraform resources our developers want.",
    "name": "David Mattia",
    "role": "Privacy Engineer"
  },
  {
    "quote": "The integration of Terragrunt into our Infrastructure as Code workflows brought about a remarkable reduction in code complexity. Previously, managing a large-scale infrastructure demanded around 20,000 lines of code, leading to challenges in code readability, maintainability, and increased likelihood of errors. Terragrunt's modular structure and the elimination of code repetition drastically streamlined our codebase, reducing it to a mere 2,000 lines. Moreover, the burden of handling variables was similarly alleviated, with the number of variable lines plummeting from 50,000 to a concise 3,000. This reduction not only enhances code maintainability but also minimizes the risk of errors, making the entire IAC process more efficient and developer-friendly. Terragrunt's ability to handle resource dependencies seamlessly meant that we no longer needed to painstakingly manage multiple backend.tf files. The hassle of coordinating and maintaining dependencies across different components of our infrastructure was significantly mitigated, leading to a more streamlined and error-resistant deployment process.",
    "name": "Shubham Tanwar",
    "role": ""
  },
  {
    "quote": "Terragrunt didn't just handle complexity; it actively simplified the process of writing and managing IaC, thanks to its emphasis on modularity. Terragrunt is not just a tool for complex projects, but a game-changer in simplifying and optimizing the way we approach Infrastructure as Code. Terragrunt has become an integral part of my toolkit in the realm of cloud architecture. Modularity in Terragrunt is not just a feature; it's a paradigm shift, enhancing the organization and reusability of code. This approach starkly contrasts with Terraform's methodology, where, although modules are used, the level of integration and reusability is not as inherently streamlined.",
    "name": "Yehor Fedorov",
    "role": "Senior DevOps Engineer"
  },
  {
    "quote": "Gruntwork's Terragrunt makes managing our infrastructure across providers and environments consistent, safe, and easy to understand.",
    "name": "Dallas Slaughter",
    "role": "Founding Engineer"
  },
  {
    "quote": "Terragrunt has allowed us to keep our code dry, speed up our deployment times, simplify our code. If you are not using Terragrunt to deploy your infrastructure you are doing it wrong.",
    "name": "Felipe Fernandes",
    "role": "Cloud Infrastructure and Devops Senior Manager"
  },
  {
    "quote": "Terragrunt has resolved some of our crucial blockers: it enabled us to use dynamic backends and dynamic providers, allowed to execute code on errors, to setup migration and emergency cleanup scripts, and to keep the code clean and consistent. Our OpenTofu modules have never looked better. Some recent changes even allowed us to leave workspaces behind. Thanks to Terragrunt's smart structuring and features, our average time of deployment went down from 30 minutes to just 3. Let Terragrunt deal with all the init work. Multiple backends? Try generating them. Huge monoliths of code that take hours to deploy? Create a stack. Repetitive code? Reuse configurations via Terragrunt.",
    "name": "Mark Prikhno",
    "role": "DevOps Engineer"
  },
  {
    "quote": "Terragrunt enabled our segmentation of Terraform state so we could move fast and scale.",
    "name": "Tobias Widen",
    "role": "Engineering Manager, Platform"
  },
  {
    "quote": "I just finished a full nested Terragrunt Stacks rewrite of our entire setup and it's so clean. This is going to do wonders for our users that aren't necessarily DevOps folks. Seriously great stuff.",
    "name": "Joshua Ward",
    "role": "Engineer"
  },
  {
    "quote": "By adopting Terragrunt Stacks, we eliminated nearly 20,000 lines of custom IaC and reduced environment plan times from two hours to just eight minutes using the Terragrunt cache provider. Less code, faster feedback loops — unlocking a whole new level of velocity for our team.",
    "name": "Lorelei Rupp",
    "role": "Senior Principal DevOps Engineer"
  }
] satisfies Testimonial[],
};
