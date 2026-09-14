---
question: "How does Drift Detection work?"
title: "How does Drift Detection work? | Terragrunt"
description: "Drift Detection runs in CI, compares your IaC against deployed cloud infrastructure, and opens a PR/MR to remediate drift back to the config defined in code."
category: "terragrunt-scale"
order: 13
---

Drift detection runs as a GitHub actions workflow / GitLab CI Pipeline at the root of your repository or within a subset (like a directory defining the infrastructure for a specific AWS account). It compares the contents of your Infrastructure as Code (IaC) code against the state of your infrastructure in your cloud environments (e.g. deployed in AWS). When it finds drift (like someone opening port 22 in the AWS console without making a corresponding change in IaC), it creates a PR/MR to remediate the drift by reverting deployed infrastructure to the designated configuration defined in IaC. You can review the impact of remediating the drift in a comment on the PR/MR, then either update the IaC to match the drift, or merge the PR/MR as is to re-apply the IaC specified configuration.
