---
headline: "Migrate from Terraform Cloud to Terragrunt Scale"
indexTitle: "Migrating from Terraform Cloud (HCP Terraform) to Terragrunt Scale"
eyebrow: "Guide"
title: "Migrating from Terraform Cloud (HCP Terraform) to Terragrunt Scale | Terragrunt"
description: "Move Terraform Cloud (HCP Terraform) workspaces to Terragrunt Scale, running plans and applies in your own CI with no resources-under-management pricing."
lede: "[**Terragrunt Scale (TGS)**](/terragrunt-scale) is a Terragrunt-native, SCM or self-hosted CI/CD pipeline including patching and drift management, built by the creators of Terragrunt. Migrating from Terraform Cloud (now **HCP Terraform**) moves your Terraform and OpenTofu runs into your own CI and off per-managed-resource pricing.

Gruntwork's solutions team runs the migration with you. Typically there is little to no state migration required for day one, and the team then helps refactor your code to use Terragrunt and Pipelines features. This guide covers the phases and what changes day to day."
---

## Why teams move from HCP Terraform to Terragrunt Scale

Teams leave HCP Terraform for three reasons. HCP Terraform is priced per managed resource, so the bill grows with the size of your estate.  It runs Terraform and does not natively support Terragrunt or OpenTofu.   And it runs plans and applies on HashiCorp-managed infrastructure by default.  Terragrunt Scale is priced without a resources-under-management (RUM) meter, understands Terragrunt natively, and runs entirely in your own CI.

| Capability | Terragrunt Scale (TGS) | HCP Terraform (Terraform Cloud) |
| --- | --- | --- |
| Native Terragrunt support | Yes: understands Terragrunt units, stacks, and the dependency graph (DAG) | No: runs Terraform, no native Terragrunt support |
| IaC engines | Terragrunt, OpenTofu, and Terraform | Terraform only; no native OpenTofu support |
| Where runs execute | Inside your own GitHub Actions or GitLab CI runners | HashiCorp-managed VMs by default |
| Where state lives | A backend you own in your own cloud (for example, an S3 bucket), reached via OIDC | HCP-managed workspaces |

## Before you begin

Migrating is mostly a change in where runs happen and how you are billed. Your cloud accounts, provider configuration, and module code stay the same.

- Admin access to your GitHub or GitLab organization, including self-hosted GitLab or GitHub Enterprise Server if you self-host.
- Your existing Terraform or OpenTofu configuration in a repository.
- Access to the cloud accounts your infrastructure runs in, so Terragrunt Scale can authenticate via OIDC.
- Your current HCP Terraform workspaces and their state, which today live in HCP-managed workspaces.

What stays the same: your cloud accounts, your providers, and your module code. What changes: runs move into your CI, state moves to a backend you own, and billing is no longer per managed resource.

## 1. Connect your repository to Terragrunt Scale

Terragrunt Scale runs as pipelines inside your own source control. Connect the repository that holds your infrastructure code.

1. Choose your SCM: GitHub, GitLab, GitHub Enterprise Server, or GitLab Self-Managed.
2. Add the Terragrunt Scale pipeline definition to the repository.
3. Configure runners: cloud-hosted runners on GitHub or GitLab, or runners you host yourself.

There is no SaaS control plane to stand up. Plans and applies execute in your runners, not on Gruntwork servers.

## 2. Point Terragrunt Scale at a backend you own

With HCP Terraform your state lives in HCP-managed workspaces.  Terragrunt Scale keeps [state in a backend you own](https://docs.terragrunt.com/features/units/state-backend/), for example an S3 bucket in your own cloud, accessed at runtime with OIDC and short-lived credentials.

1. Create or choose the state backend in your own cloud.
2. Configure OIDC between your CI and your cloud so runs get temporary, least-privilege credentials instead of long-lived secrets.
3. Let Gruntwork's solutions team scope the state move for your setup. Typically there is little to no state migration required for day one.

## 3. Move plans and applies into your CI

Terragrunt Scale is PR-driven. Instead of queuing runs in a hosted service, you open a pull request and the pipeline plans the affected units; merging applies them.

1. On a pull request, Terragrunt Scale runs a plan and posts it for review.
2. On merge, it applies the change.
3. Because it understands
  
  Terragrunt units, stacks
  
  , and the dependency graph (DAG), it plans and applies only the affected units to reduce blast radius.

## 4. Cut over and retire your HCP Terraform runs

Run both in parallel until you trust the new pipeline, then stop triggering runs in HCP Terraform.

1. Validate a plan and apply through Terragrunt Scale against a non-critical workspace first.
2. Confirm state reads and writes against your own backend.
3. Stop running plans and applies in HCP Terraform once the equivalent pipeline is live.

## 5. Refactor to use Terragrunt and Pipelines features

Day one keeps your configuration close to what you had. After cutover, Gruntwork's solutions team helps refactor the code to take advantage of Terragrunt and Pipelines features to clean up the code and improve readability and scalability.

- Introduce Terragrunt units and stacks to remove duplication.
- Add
  
  drift detection
  
  , which opens a PR/MR to remediate when live infrastructure diverges from code.
- Adopt automated dependency and version updates.

## What you have after the migration

Your Terraform and OpenTofu workflows run as PR-driven Terragrunt Scale pipelines in your own CI. Your state lives in a backend you own, reached via OIDC. You are billed without a per-managed-resource meter, and Terragrunt Scale is cloud-agnostic, so the same setup works across AWS, GCP, and Azure.

## Do I have to migrate my state?

Typically there is little to no state migration required for day one. Gruntwork's solutions team runs the migration with you and scopes the state move for your setup. Terragrunt Scale keeps state in a backend you own, such as an S3 bucket, rather than in a hosted workspace.

## Will my existing Terraform and OpenTofu code work?

Yes. Terragrunt Scale runs Terragrunt, OpenTofu, and Terraform. Your provider configuration and module code stay the same; what changes is where runs execute and how state is stored.

## Does Terragrunt Scale use resources-under-management (RUM) pricing like HCP Terraform?

No. Terragrunt Scale has no RUM pricing and no meter. The Free and Team tiers are limited by the number of Terragrunt Units deployed, and Enterprise pricing is custom. HCP Terraform is priced per managed resource.

## Where do my runs execute after migrating?

Inside your own GitHub Actions or GitLab CI runners. There is no SaaS control plane, and plans and applies run in your runners, not on Gruntwork servers. HCP Terraform runs on HashiCorp-managed infrastructure by default.

## Is HCP Terraform the same as Terraform Cloud?

Yes. HashiCorp renamed Terraform Cloud to HCP Terraform, effective April 22, 2024.

## Can I roll back during the migration?

Yes. Run Terragrunt Scale in parallel with HCP Terraform and validate against a non-critical workspace before you retire your HCP Terraform runs. Your cloud accounts and module code are unchanged during cutover.
