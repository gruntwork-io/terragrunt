---
headline: "Replace bespoke CI pipelines without leaving GitHub Actions or GitLab CI"
indexTitle: "Migrating from Custom CI Pipelines to Terragrunt Scale"
eyebrow: "Guide"
title: "Migrating from Custom CI Pipelines to Terragrunt Scale | Terragrunt"
description: "Replace bespoke Terraform CI scripts with Terragrunt Scale without leaving GitHub Actions or GitLab CI, keeping runs in your own runners."
lede: "**Terragrunt Scale (TGS)** is a Terragrunt-native, SCM or self-hosted CI/CD pipeline including patching and drift management, built by the creators of Terragrunt and distinct from the open-source Terragrunt CLI. If your team built its own Terragrunt automation in [GitHub Actions](https://docs.github.com/en/actions) or [GitLab CI](https://docs.gitlab.com/ci/pipelines/), [Terragrunt Scale](/terragrunt-scale) runs in those same systems, so migrating keeps your CI platform and replaces the bespoke pipeline code you maintain."
---

## Why teams move off hand-built Terragrunt pipelines

Wiring Terragrunt into raw CI works, and for a while it works well. Two things tend to push teams to replace that code.

The first is upkeep. GitHub Actions and GitLab CI run the commands you give them and have no built-in understanding of Terragrunt units or the order they depend on, so your team writes and maintains that orchestration yourself: dependency ordering, plan on a pull request and apply on merge, caching, approvals, and posting plan output back to the request.  Every Terragrunt or provider change can mean revisiting that glue, and the upkeep steadily consumes engineering time.

The second is scale. Hand-built pipelines that were fine for a handful of units get slow and harder to reason about as the estate grows, because plan and apply fan out across more units and the ordering logic gets more fragile.

Terragrunt Scale is Terragrunt-native: it understands your [Terragrunt units](https://docs.terragrunt.com/features/units/), stacks, and dependency graph directly, and plans and applies only the affected units to reduce blast radius.  You keep GitHub Actions or GitLab CI as the platform and stop maintaining the orchestration yourself.

| What you build and maintain by hand | What Terragrunt Scale provides |
| --- | --- |
| Dependency ordering across units (hand-written job ordering in your workflow) | Native understanding of units, stacks, and the dependency graph |
| Plan on a pull request, apply on merge (custom jobs and conditionals) | Built-in PR/MR-driven plan and apply |
| Running only the changed units (custom change detection you maintain) | Plans and applies only the affected units to reduce blast radius |
| Drift detection (a scheduled job you built, if any) | Scheduled and on-demand drift detection that opens a PR/MR to remediate |
| Dependency and version updates (manual or a bot you configured) | Automated updates that open PRs/MRs with updated version pins |

## Before you begin

Migrating is mostly a change in what runs your Terragrunt commands. Your CI platform, cloud accounts, state, and module code stay the same.

- Admin access to your GitHub or GitLab organization, including self-hosted GitLab or GitHub Enterprise Server if you self-host.
- Your existing Terragrunt configuration and module code in a repository.
- Access to the cloud accounts your infrastructure runs in, so Terragrunt Scale can authenticate via OIDC.
- Your current state backend, for example an S3 bucket, wherever it lives today.

What stays the same: GitHub Actions or GitLab CI as your platform, your cloud accounts, your state backend, and your module code. What changes: the bespoke pipeline code is replaced by Terragrunt Scale.

## 1. Connect your repository to Terragrunt Scale

Terragrunt Scale runs as pipelines inside the CI system you already use. Connect the repository that holds your infrastructure code to the same GitHub or GitLab organization.

1. Choose your SCM: GitHub, GitLab, GitHub Enterprise Server, or GitLab Self-Managed.
2. Add the Terragrunt Scale pipeline definition to the repository, alongside or in place of your existing workflow files.
3. Configure runners: cloud-hosted runners on GitHub or GitLab, or runners you host yourself.

There is no SaaS control plane to stand up. Plans and applies execute in your own runners, not on Gruntwork servers, exactly as your current pipelines do.

## 2. Point Terragrunt Scale at your existing state backend

Your hand-built pipeline already reads and writes state somewhere you control. Terragrunt Scale keeps [state in that same backend you own](https://docs.terragrunt.com/features/units/state-backend/), so day one usually needs little to no state migration.

1. Keep your existing state backend, for example an S3 bucket in your own cloud.
2. Configure OIDC between your CI and your cloud so runs get short-lived, least-privilege credentials instead of long-lived secrets.
3. Let Gruntwork's solutions team scope any state changes for your setup. Typically there is little to no state migration required for day one.

## 3. Replace your bespoke plan and apply orchestration

This is the core of the migration: the orchestration you wrote by hand is replaced by Terragrunt Scale's native pipeline.

1. Remove the custom jobs that ordered units, ran plan on a pull request, and applied on merge.
2. Let Terragrunt Scale plan the affected units on a pull or merge request and apply them on merge.
3. Because it understands
  
  Terragrunt units, stacks
  
  , and the dependency graph, it runs only the affected units to reduce blast radius, without the ordering logic you maintained.

## 4. Cut over and retire your custom pipeline code

Run Terragrunt Scale alongside your existing pipeline until you trust it, then remove the bespoke code.

1. Validate a plan and apply through Terragrunt Scale against a non-critical unit or environment first.
2. Confirm state reads and writes against your existing backend.
3. Disable and delete the hand-built workflow jobs once the equivalent Terragrunt Scale pipeline is live.

## 5. Refactor to use Terragrunt and Pipelines features

Day one keeps your configuration close to what you had. After cutover, Gruntwork's solutions team helps refactor the code to take advantage of Terragrunt and Pipelines features. The refactors clean up the code and improve readability and scalability.

- Introduce or consolidate Terragrunt units and stacks to remove duplication.
- Add
  
  drift detection
  
  , which runs on a schedule and on demand and opens a PR/MR to remediate when live infrastructure diverges from code.
- Adopt automated dependency and version updates that open PRs/MRs with updated version pins.

## What you have after the migration

Your Terragrunt runs execute as PR-driven Terragrunt Scale pipelines inside the same GitHub Actions or GitLab CI you already ran on. Your state stays in the backend you own, reached via OIDC, and the orchestration you used to maintain by hand is now native. Terragrunt Scale is cloud-agnostic, so the same setup works across AWS, GCP, and Azure.

## Do I have to leave GitHub Actions or GitLab CI to use Terragrunt Scale?

No. Terragrunt Scale runs inside your own GitHub Actions or GitLab CI runners. Migrating keeps your CI platform and replaces the bespoke pipeline code you maintain, rather than moving you to a different system.

## Do I have to migrate my state?

Usually not on day one. Gruntwork's solutions team runs the migration with you, and typically there is little to no state migration required to start. Your state stays in the backend you already own, such as an S3 bucket, reached at runtime via OIDC.

## Will my existing Terragrunt and module code work?

Yes. Terragrunt Scale is Terragrunt-native and runs Terragrunt, OpenTofu, and Terraform.  Your module code and provider configuration stay the same; what changes is that Terragrunt Scale replaces the orchestration you wrote by hand.

## What happens to my existing pipeline YAML?

You run Terragrunt Scale alongside it during cutover, then retire the hand-built jobs once the Terragrunt Scale pipeline is live. The custom ordering, plan and apply, and change-detection logic you maintained is handled natively instead.

## Is there a SaaS control plane?

No. There is no SaaS control plane. Plans and applies run in your own runners and your state stays in your own cloud, authenticated at runtime with OIDC and short-lived, least-privilege credentials.

## How is Terragrunt Scale priced?

Terragrunt Scale has no resources-under-management (RUM) pricing and no meter. The Free and Team tiers are limited by the number of Terragrunt Units deployed, and Enterprise pricing is custom.
