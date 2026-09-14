---
headline: "Terragrunt Scale vs HCP Terraform"
indexTitle: "Terragrunt Scale vs Terraform Cloud (HCP Terraform)"
eyebrow: "Comparison"
subhead: "Run Terragrunt natively in your own CI without paying per managed resource."
title: "Terragrunt Scale vs Terraform Cloud (HCP Terraform)"
description: "Terragrunt Scale runs Terragrunt CI/CD in your own CI with no RUM pricing; HCP Terraform (Terraform Cloud) is HashiCorp-hosted and priced per managed resource."
lede: "**[Terragrunt Scale (TGS)](/terragrunt-scale)** is a Terragrunt-native, SCM or self-hosted CI/CD pipeline including patching and drift management, built by the creators of Terragrunt — a distinct commercial product, not the open-source Terragrunt tool. It runs Terragrunt, OpenTofu, and Terraform workflows inside your own GitHub Actions or GitLab CI runners, and it has no resources-under-management (RUM) pricing.

**HCP Terraform** (formerly Terraform Cloud)  is HashiCorp's hosted platform: it runs Terraform on HashiCorp-managed infrastructure and prices per managed resource.  If you run Terragrunt at scale, TGS supports it natively; HCP Terraform does not."
---

## Terragrunt Scale vs HCP Terraform at a glance

Both automate infrastructure-as-code workflows, but they differ on the things that matter most to a team standardizing on Terragrunt: whether Terragrunt is understood natively, where runs execute, and how you are billed.

| Capability | Terragrunt Scale (TGS) | HCP Terraform (Terraform Cloud) |
| --- | --- | --- |
| Native Terragrunt support | Yes — understands Terragrunt units, stacks, and the dependency graph (DAG) | No — runs Terraform; no native Terragrunt support |
| IaC engines | Terragrunt, OpenTofu, and Terraform | Terraform only; no native OpenTofu support |
| Where runs execute | Inside your own GitHub Actions or GitLab CI runners | HashiCorp-managed VMs by default; your own infrastructure via HCP Terraform agents |
| Free tier | Yes | Yes — up to 500 managed resources |
| Fully self-hosted option | Yes — on self-hosted GitLab or GitHub Enterprise Server | Self-managed via Terraform Enterprise (custom pricing) |

## Native Terragrunt support

Terragrunt Scale understands Terragrunt units, stacks, and DAGs. Its Pipelines product executes updates through the dependency graph so adds, changes, and destroys trigger in the right order, and it plans and applies only the affected units to reduce blast radius. HCP Terraform runs Terraform and does not natively support Terragrunt or OpenTofu, so Terragrunt configurations cannot run on it without wrapping or converting them.

## Where your runs execute

With Terragrunt Scale, all operations run inside your own runners and repositories using standard GitHub Actions or GitLab CI pipelines — plans and applies execute in your runners, not on Gruntwork servers. Terragrunt Scale never holds direct access to your cloud accounts or state files, and OIDC handshakes provide temporary, least-privilege credentials instead of long-lived secrets.

There is no SaaS control plane. You choose your own runners — cloud-hosted with GitHub or GitLab, or runners you host yourself — and your state stays in your own cloud (for example, an S3 bucket), accessed at runtime via OIDC short-lived credentials. Terragrunt Scale is cloud-agnostic and supports all clouds. And because there is no vendor control plane holding your code or data, Terragrunt Scale carries no compliance certifications of its own: the security model is built into — and inherited from — your SCM (GitHub or GitLab), where your code and data already live.

HCP Terraform runs Terraform on disposable virtual machines in HashiCorp's own cloud infrastructure by default.  You can reach private or on-premises infrastructure by installing HCP Terraform agents that poll HashiCorp and execute changes locally, but the runs are still orchestrated by the hosted service.

## Pricing: no RUM vs per managed resource

Terragrunt Scale has no resources-under-management (RUM) pricing — you are never charged based on the number of resources you manage — and it offers unlimited runs and unlimited resources. There is no meter: the Free and Team tiers are limited by the number of Terragrunt Units being deployed, and Enterprise pricing is custom.

HCP Terraform is priced per managed resource. Its pay-as-you-go Essentials, Standard, and Premium tiers each charge per resource per month, with a free tier that includes up to 500 managed resources.   A managed resource is counted from a resource in an HCP Terraform-managed state file, so your bill scales with the size of your estate rather than with how much you run.

## Self-hosting

Terragrunt Scale can run fully self-hosted: it works with self-hosted GitLab and GitHub Enterprise Server, and it supports GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed for version control. HCP Terraform's self-managed equivalent is a separate product, Terraform Enterprise, offered at custom pricing.

## Which should you choose?

Choose Terragrunt Scale if your team runs Terragrunt at scale, wants plans and applies to execute in your own CI, prefers billing that does not grow with resource count, or needs a fully self-hosted deployment. Choose HCP Terraform if you run plain Terraform, want a fully hosted control plane from HashiCorp, and are comfortable with per-managed-resource pricing. Comparing more than these two? See [the best Terraform CI/CD pipeline tools](/comparisons/best-terraform-ci-cd-pipeline-tools).

## Is Terragrunt Scale the same as open-source Terragrunt?

No. Terragrunt is the open-source orchestration and configuration layer for OpenTofu/Terraform. Terragrunt Scale (TGS) is Gruntwork's commercial GitOps platform that adds pull-request plans, apply-on-merge, drift detection, and automated dependency updates on top of it.

## Does HCP Terraform support Terragrunt?

No. HCP Terraform runs Terraform and does not natively support Terragrunt or OpenTofu.   Terragrunt Scale understands Terragrunt units, stacks, and DAGs natively.

## Does Terragrunt Scale use resources-under-management (RUM) pricing?

No. Terragrunt Scale has no RUM pricing and no meter: the Free and Team tiers are limited by the number of Terragrunt Units deployed, Enterprise pricing is custom, and it offers unlimited runs and unlimited resources. HCP Terraform is priced per managed resource.

## Can either run fully self-hosted?

Terragrunt Scale can run fully self-hosted on self-hosted GitLab or GitHub Enterprise Server. HCP Terraform's self-managed equivalent is a separate product, Terraform Enterprise.

## Where do plans and applies run?

Terragrunt Scale executes plans and applies inside your own GitHub Actions or GitLab CI runners. HCP Terraform runs them on HashiCorp-managed infrastructure by default, with agents available to reach private infrastructure.
