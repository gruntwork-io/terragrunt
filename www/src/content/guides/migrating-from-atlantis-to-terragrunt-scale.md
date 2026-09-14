---
headline: "Migrate from Atlantis to Terragrunt Scale"
indexTitle: "Migrating from Atlantis to Terragrunt Scale"
eyebrow: "Guide"
title: "Migrating from Atlantis to Terragrunt Scale | Terragrunt"
description: "Move from a self-hosted Atlantis server to Terragrunt Scale while keeping the pull-request workflow your team already uses, usually with little to no state migration."
lede: "**Terragrunt Scale (TGS)** is a Terragrunt-native, SCM or self-hosted CI/CD pipeline with patching and drift management, built by the creators of Terragrunt and distinct from the open-source Terragrunt CLI.

Teams move to [Terragrunt Scale](/terragrunt-scale) from Atlantis when their infrastructure estate grows, the operational burden of the self-hosted Atlantis server grows with it, and operating at scale becomes a struggle. The migration to TGS keeps the pull-request workflow your team already uses, and on day one, it usually needs little to no state migration."
---

## Why teams move off Atlantis

Atlantis is a genuinely good fit for straightforward Terraform. It is free and open source under Apache 2.0, so it costs nothing to run beyond your own compute.  It is a single self-hosted Go application that listens for pull-request webhooks and comments terraform plan and apply output back on the PR,  it works with GitHub, GitLab, Gitea, Bitbucket, and Azure DevOps,  and it is a community-maintained CNCF Sandbox project.  If Atlantis covers your needs and you are happy operating it, there is no reason to move.

The biggest reason teams migrate off Atlantis is scale. As the infrastructure estate grows, the operational burden of self-hosting Atlantis grows with it, and operating at scale becomes a struggle.  Two pressures feed that burden. The first is maintenance: running Atlantis yourself means owning the server, its image, upgrades, and access. And that work grows with the estate the server manages. The second is Terragrunt support. Atlantis runs Terragrunt only through custom workflows, and it does not ship the terragrunt binary, so you add it to the Atlantis image yourself and maintain it there.  Terragrunt Scale is Terragrunt-native instead: it understands your [Terragrunt units](https://docs.terragrunt.com/features/units/), stacks, and dependency graph directly.

Caddi, an automation startup, weighed Atlantis and chose Gruntwork Pipelines. Founding engineer Dallas Slaughter had spent years administering Atlantis in prior roles, where keeping it going "was almost a part-time job."  On running Terragrunt through Atlantis he was blunt: "Once you add Terragrunt in the mix ... it's a no go."

| Stage | What happens |
| --- | --- |
| Day one | Little to no state migration. Your state stays where it lives, Terragrunt Scale runs in your own CI and authenticates via OIDC, and you keep the PR/MR workflow. |
| After cutover | Gruntwork's solutions team assists with refactors that use Terragrunt and Pipelines features to clean up the code and improve readability and scalability. |
| Ongoing | Scheduled and on-demand [drift detection](https://docs.terragrunt.com/terragrunt-scale/drift-detection/) opens PRs/MRs to remediate drift, and automated dependency updates open PRs/MRs with updated version pins. |

## Before you begin

Migrating is mostly a change in where runs happen, not in what your code manages. Your cloud accounts, provider configuration, and module code stay the same.

- Admin access to your GitHub or GitLab organization, including self-hosted GitLab or GitHub Enterprise Server if you self-host.
- Your existing Terraform or Terragrunt configuration in a repository, including anything you run today through Atlantis.
- Access to the cloud accounts your infrastructure runs in, so Terragrunt Scale can authenticate via OIDC.
- Your existing state backend, for example an
  
  S3 bucket you own
  
  , which stays where it is.

## 1. Connect your repository to Terragrunt Scale

Like Atlantis, Terragrunt Scale drives every infrastructure change through a pull or merge request in your own repositories, so the operating model your team already uses carries over. Connect the repository that holds your infrastructure code.

1. Choose your CI: GitHub Actions or GitLab CI, self-hosted or cloud-hosted.
2. Add the Terragrunt Scale pipeline definition to the repository.
3. Configure runners in your own CI. There is no standalone Atlantis server to keep running and no SaaS control plane in the path.

## 2. Point Terragrunt Scale at your existing state backend

Your OpenTofu or Terraform state stays where it already lives. On day one there is typically little to no state migration required.

1. Keep your state backend in place, for example an S3 bucket in your own cloud.
2. Configure OIDC between your CI and your cloud so runs get short-lived, least-privilege credentials instead of long-lived secrets.
3. Let Gruntwork's solutions team scope any state move for your setup.

## 3. Cut over from your Atlantis server

Run both in parallel until you trust the new pipeline, then retire Atlantis.

1. Validate a plan and apply through Terragrunt Scale against a non-critical unit first.
2. Confirm state reads and writes against your own backend.
3. Stop your Atlantis server once the equivalent Terragrunt Scale pipeline is live.

## 4. Refactor to use Terragrunt and Pipelines features

Day one keeps your configuration close to what you had. After cutover, Gruntwork's solutions team helps refactor the code to take advantage of Terragrunt and Pipelines features to clean up the code and improve readability and scalability.

- Introduce Terragrunt units and stacks to remove duplication.
- Add
  
  drift detection
  
  , which opens a PR/MR to remediate when live infrastructure diverges from code.
- Adopt automated dependency updates that open PRs/MRs with updated version pins.

## What you have after the migration

Your pull-request workflow runs as Terragrunt Scale pipelines in your own GitHub Actions or GitLab CI runners. Your state stays in a backend you own, reached via OIDC, and there is no standalone Atlantis server to maintain and no SaaS control plane in the path. Scheduled and on-demand drift detection opens PRs/MRs to remediate drift, and automated dependency updates open PRs/MRs with updated version pins.

Caddi went from evaluating Atlantis to running on Gruntwork Pipelines, the CI/CD pipeline at the core of Terragrunt Scale, in a single short session. "We were on a call for 30 or 45 minutes, and we were up and running," Slaughter said. "We went from local, opaque applies to a shared, auditable pipeline in under an hour." Since then it has stayed out of the way: "I haven't touched the config once since we set it up. It's just there. It just works."

## Do I have to migrate my state to move from Atlantis to Terragrunt Scale?

Usually not on day one. Gruntwork's solutions team runs the migration with you, and typically there is little to no state migration required to start: your state stays where it already lives and Terragrunt Scale authenticates to it at runtime through OIDC.

## Does Terragrunt Scale keep the pull-request workflow Atlantis uses?

Yes. Both tools drive infrastructure changes through pull or merge requests in your own repositories. The main change is that Terragrunt Scale runs in your own GitHub Actions or GitLab CI runners instead of a standalone Atlantis server you maintain.

## Why do teams outgrow Atlantis when they use Terragrunt?

As the infrastructure estate grows, the operational burden of the self-hosted Atlantis server grows with it, and Terragrunt adds to that burden. Atlantis supports Terragrunt only through custom workflows and does not ship the terragrunt binary, so you add and maintain it in the Atlantis image yourself. Terragrunt Scale is Terragrunt-native and understands units, stacks, and the dependency graph directly.

## Is Atlantis still a good choice?

Yes, for simpler setups. Atlantis is free and open source under Apache 2.0, self-hosted, and works with a broad set of version-control systems. It is worth keeping if that is all you need and you are comfortable operating it.

## Does the migration require a SaaS control plane?

No. There is no SaaS control plane. Terragrunt Scale runs in your own runners and your state stays in your own cloud, authenticated at runtime with OIDC and short-lived, least-privilege credentials.

## How is Terragrunt Scale priced next to free Atlantis?

Atlantis is free and open source; you pay to host and operate it. Terragrunt Scale has no resources-under-management (RUM) pricing and no meter: the Free and Team tiers are limited by the number of Terragrunt Units deployed, and Enterprise pricing is custom.
