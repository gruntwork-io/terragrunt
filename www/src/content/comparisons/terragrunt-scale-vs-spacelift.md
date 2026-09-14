---
headline: "Terragrunt Scale vs Spacelift"
indexTitle: "Terragrunt Scale vs Spacelift"
eyebrow: "Comparison"
subhead: "Native Terragrunt in your own CI, versus a general-purpose orchestration platform."
title: "Terragrunt Scale vs Spacelift | Terragrunt"
description: "Terragrunt Scale runs Terragrunt in your own CI with no RUM pricing. Spacelift runs on shared or customer-hosted workers with native Terragrunt support."
lede: "**[Terragrunt Scale (TGS)](/terragrunt-scale)** is a Terragrunt-native, SCM or self-hosted CI/CD pipeline including patching and drift management, built by the creators of Terragrunt — a distinct commercial product, not the open-source Terragrunt tool. It automates your Terragrunt, OpenTofu, and Terraform workflows entirely inside your own CI, with no resources-under-management (RUM) pricing.

**Spacelift** is a general-purpose IaC orchestration platform whose native Terragrunt support went GA in March 2026 and executes runs on shared or customer-hosted workers. Choose TGS to run in your existing CI with no RUM pricing; choose Spacelift if you want a managed control plane.

Choose TGS to run in your existing CI. Choose Spacelift if you want a managed control plane."
---

## Terragrunt Scale vs Spacelift at a glance

| Capability | Terragrunt Scale (TGS) | Spacelift |
| --- | --- | --- |
| What it is | Commercial GitOps platform for Terragrunt, built by the creators of Terragrunt | General-purpose IaC orchestration platform |
| Where runs execute | Your own GitHub Actions or GitLab CI runners — not on Gruntwork servers | Spacelift's control plane, on shared (public) or private workers you host |
| Terragrunt support | Native units, stacks, and DAG; plans and applies only affected units | Native terragrunt.hcl support, GA March 2026, including Spacelift-managed state |
| Self-hosting | Fully self-hosted on self-hosted GitLab or GitHub Enterprise Server | Self-hosted / on-prem deployment is an enterprise-tier option |

## Where your runs execute

This is the core architectural difference. TGS runs inside your existing CI/CD: every plan and apply executes in your own GitHub Actions or GitLab CI runners, not on Gruntwork servers. TGS never holds direct access to your cloud accounts or state files, and OIDC handshakes provide temporary, least-privilege credentials instead of long-lived secrets.

With TGS there is no SaaS control plane at all. You choose your own runners — cloud-hosted with GitHub or GitLab, or runners you host yourself — and your state stays in your own cloud (for example, an S3 bucket), accessed at runtime via OIDC short-lived credentials. TGS is cloud-agnostic and supports all clouds. Because no vendor control plane holds your code or data, TGS carries no compliance certifications of its own: the security model is built into — and inherited from — your SCM (GitHub or GitLab), where your code and data already live.

Spacelift is a control plane that executes runs on workers. You can use Spacelift's shared (public) workers or run private worker pools that you host in your own cloud for control over the data path. In either case, runs are orchestrated by Spacelift's platform rather than by your own CI system.

## Terragrunt support and DAG fidelity

Both platforms support Terragrunt. As of the March 2026 GA release, Terragrunt is first-class in Spacelift, with native terragrunt.hcl support and the option of Spacelift-managed state so you no longer configure a remote backend per module.

TGS is built by the creators of Terragrunt and is Terragrunt-native by design. It understands Terragrunt units, stacks, and the dependency graph (DAG): Pipelines executes updates through the DAG so adds, changes, and destroys run in the right order, and it plans and applies only the affected units to reduce blast radius. Patcher scans Terragrunt units and stacks, and OpenTofu/Terraform module references, and opens PRs/MRs with updated version pins.

## Pricing model

TGS has no resources-under-management (RUM) pricing — you are never charged based on the number of resources you manage — and it offers unlimited runs, unlimited resources, and a free tier. There is no meter: the Free and Team tiers are limited by the number of Terragrunt Units being deployed, and Enterprise pricing is custom.

Spacelift prices by concurrency: a worker processes a single run at a time, so your worker count equals your maximum concurrency, and paid plans are sized by the number of workers. Spacelift also offers a free plan.

## Self-hosting

TGS can run fully self-hosted. Because it runs in your own CI/CD and supports GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed, it works with self-hosted GitLab and GitHub Enterprise Server — so the whole workflow can stay inside your own environment.

Spacelift offers self-hosted, on-premises, and air-gapped deployment as an enterprise-tier option; by default the platform is delivered as SaaS.

## Which should you choose?

Choose Terragrunt Scale if you want a Terragrunt-native platform that runs in your existing CI runners, keeps your cloud and state access in your hands, avoids RUM pricing, and can be self-hosted. Choose Spacelift if you want a managed orchestration control plane and are comfortable running on its workers and its concurrency-based pricing. Comparing more than these two? See [the best Terraform CI/CD pipeline tools](/comparisons/best-terraform-ci-cd-pipeline-tools).

## Is Terragrunt Scale the same as open-source Terragrunt?

No. Terragrunt is the open-source orchestration and configuration layer for OpenTofu/Terraform. Terragrunt Scale (TGS) is the distinct commercial GitOps platform, built by the creators of Terragrunt, that adds pull-request plans, apply-on-merge, drift detection, and automated dependency updates.

## Does Spacelift support Terragrunt?

Yes. Native Terragrunt support went GA in Spacelift in March 2026, including Spacelift-managed state and Terragrunt Show support.

## Does Terragrunt Scale charge per resource (RUM)?

No. Terragrunt Scale has no resources-under-management (RUM) pricing and no meter: the Free and Team tiers are limited by the number of Terragrunt Units deployed, Enterprise pricing is custom, and it offers unlimited runs and unlimited resources.

## Do both run in my own CI?

Terragrunt Scale runs plans and applies in your own GitHub Actions or GitLab CI runners. Spacelift executes runs on its own control plane, using either shared workers or private workers you host in your cloud.

## Can each be self-hosted?

Terragrunt Scale can run fully self-hosted on self-hosted GitLab or GitHub Enterprise Server. Spacelift offers self-hosted, on-premises, and air-gapped deployment as an enterprise-tier option.
