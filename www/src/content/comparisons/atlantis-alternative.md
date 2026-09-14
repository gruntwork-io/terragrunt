---
headline: "A commercially maintained, Terragrunt-native alternative to Atlantis"
indexTitle: "Terragrunt Scale as an Atlantis Alternative"
eyebrow: "Comparison"
subhead: "Terragrunt-native pipelines, drift remediation, and commercial support for teams that have outgrown Atlantis."
title: "Terragrunt Scale: A Terragrunt-Native Atlantis Alternative"
description: "Terragrunt Scale is a commercially maintained, Terragrunt-native GitOps platform with built-in drift detection and remediation PRs — an alternative to Atlantis."
lede: "**[Terragrunt Scale (TGS)](/terragrunt-scale)** is a Terragrunt-native, SCM or self-hosted CI/CD pipeline including patching and drift management. If your team has outgrown Atlantis, TGS adds native Terragrunt support, scheduled drift detection that opens remediation PRs, and automated dependency updates, while still running in your own CI."
---

## Terragrunt Scale vs Atlantis at a glance

Both tools drive infrastructure changes through pull or merge requests in your own repositories. The difference is scope and support. Atlantis is a free, self-hosted project focused on running terraform plan and apply from PR comments. Terragrunt Scale is a commercially maintained platform built around Terragrunt units, stacks, and dependency graphs, with drift detection and dependency updates included.

| Capability | Terragrunt Scale (TGS) | Atlantis |
| --- | --- | --- |
| What it is | Commercial GitOps platform, built by the creators of Terragrunt | Open-source (Apache 2.0), community-maintained CNCF Sandbox project |
| Core workflow | PR/MR plans, apply on merge, across Terragrunt units, stacks, and the DAG | Comments terraform plan and apply output back on pull requests |
| Terragrunt support | Native — understands units, stacks, and DAGs | Via custom workflows; the terragrunt binary is not shipped and must be added to the Atlantis image |
| Drift detection | Scheduled and on-demand scans that open a PR/MR to remediate | Alpha, flag-gated API; no built-in scheduler; remediation plan-only by default |
| Version control | GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed | GitHub, GitLab, Gitea, Bitbucket, and Azure DevOps |

## What Atlantis does well

Atlantis is genuinely good at what it was built for, and it is worth keeping if that is all you need.

- Free and open source.
  
  Atlantis is licensed under Apache 2.0 and costs nothing to run beyond your own compute.
- Self-hosted and simple.
  
  It is a single self-hosted Go application that listens for pull-request webhooks and comments terraform plan and apply output back on the PR.
- Broad version-control support.
  
  It works with GitHub, GitLab, Gitea, Bitbucket, and Azure DevOps.
- Community-governed.
  
  Atlantis is a CNCF Sandbox project maintained by its community.

## Where Terragrunt Scale is different

Terragrunt Scale (TGS) is a distinct commercial product — not the open-source Terragrunt CLI. It is built and maintained by the creators of Terragrunt and packages the workflows teams otherwise assemble and maintain themselves.

- Native Terragrunt support.
  
  TGS understands Terragrunt units, stacks, and DAGs, and plans and applies only the affected units in dependency order. With Atlantis, Terragrunt runs through custom workflows and you add the terragrunt binary to the Atlantis image yourself.
- Built-in drift detection with remediation PRs.
  
  TGS runs scheduled and on-demand scans and, when it finds drift, opens a PR/MR to remediate it. Atlantis has added an alpha drift-detection API behind the --enable-drift-detection flag, but ships no built-in scheduler, and remediation is plan-only by default.
- Automated dependency updates.
  
  Patcher scans Terragrunt units and stacks and OpenTofu/Terraform module references and opens PRs/MRs with updated version pins. This is outside the scope of the Atlantis plan/apply loop.
- Least-privilege cloud auth.
  
  Operations run in your own GitHub Actions or GitLab CI runners, and OIDC handshakes provide temporary, least-privilege credentials, so TGS never holds direct access to your cloud accounts or state.
- Commercially maintained.
  
  TGS is a supported product with a vendor behind it. Atlantis relies on community maintenance.

## What switching looks like

Caddi, an automation startup, chose Gruntwork Pipelines — the CI/CD pipeline at the core of Terragrunt Scale — over Atlantis. Running Atlantis themselves "was almost a part-time job," says founding engineer Dallas Slaughter, and "once you add Terragrunt in the mix … it's a no go." Pipelines setup took under an hour: "We were on a call for 30 or 45 minutes, and we were up and running." The result, in Slaughter's words: "We went from local, opaque applies to a shared, auditable pipeline in under an hour" — and since then, "I haven't touched the config once since we set it up. It's just there. It just works."

## Pricing

Atlantis is free and open source; you pay only to host and operate it. Terragrunt Scale offers a free tier with unlimited runs and unlimited resources, plus paid plans, and has no resources-under-management (RUM) pricing — you are never charged by the number of resources you manage. There is no meter: the Free and Team tiers are limited by the number of Terragrunt Units being deployed, and Enterprise pricing is custom.

## When to choose which

Choose Atlantis if you want a free, self-hosted PR-automation server for straightforward Terraform and are comfortable maintaining it and wiring up Terragrunt yourself. Choose Terragrunt Scale if you are scaling Terragrunt across many units, stacks, environments, and accounts and want native Terragrunt support, scheduled drift remediation, and dependency updates as a maintained product — still running in your own CI. Comparing more than these two? See [the best Terraform CI/CD pipeline tools](/comparisons/best-terraform-ci-cd-pipeline-tools).

## Is Terragrunt Scale the same as open-source Terragrunt?

No. Terragrunt Scale (TGS) is a commercial GitOps platform built by the creators of Terragrunt. Open-source Terragrunt is the orchestration and configuration layer; TGS adds pull-request plans, apply-on-merge, drift detection, and automated dependency updates on top of it.

## Does Atlantis support Terragrunt?

Yes, but not natively. You enable Terragrunt through Atlantis custom workflows, and you must add the terragrunt binary to the Atlantis image or server yourself, since Atlantis does not ship with it.

## Does Atlantis have drift detection?

Atlantis has added alpha drift-detection API endpoints, enabled with the --enable-drift-detection flag, but there is no built-in scheduler — you trigger scans from external cron or CI — and remediation is plan-only by default. Terragrunt Scale runs scheduled and on-demand scans and opens a PR/MR to remediate drift.

## Is Terragrunt Scale free like Atlantis?

Atlantis is free and open source under Apache 2.0. Terragrunt Scale has a free tier with unlimited runs and unlimited resources and no resources-under-management pricing or meter — the Free and Team tiers are limited by the number of Terragrunt Units deployed, and Enterprise pricing is custom.

## What changes when moving from Atlantis to Terragrunt Scale?

Both tools drive changes through pull or merge requests in your own repositories, so the model is familiar. Terragrunt Scale runs in your own GitHub Actions or GitLab CI runners rather than a standalone server, and understands your Terragrunt units and stacks directly.
