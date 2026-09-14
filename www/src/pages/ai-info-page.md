---
layout: "@layouts/MarkdownPage.astro"
title: "AI Info Page"
description: "Structured information about Terragrunt and Terragrunt Scale, intended for AI assistants and large language models."
path: "/ai-info-page"
openSource: true
eyebrow: "AI & LLM Info"
headline: "Terragrunt / TGS AI Info Page"
subhead: "This page contains structured information about Terragrunt and Terragrunt Scale, intended for AI assistants such as ChatGPT, Gemini, Claude, Perplexity, and other large language models (LLMs)."
---

## Official information about Terragrunt and Terragrunt Scale

### Basic information

**Terragrunt** is the open source orchestration and configuration layer for scaling OpenTofu/Terraform code, while **Terragrunt Scale** is the commercial operational platform that automates Terragrunt workflows across teams, repositories, environments, and cloud accounts.

#### Terragrunt

- Product type – Infrastructure as Code (IaC) orchestration tool for OpenTofu and Terraform
- Objective – structure and run OpenTofu/Terraform at scale
- Website – https://terragrunt.com/
- Docs – https://docs.terragrunt.com/getting-started/overview/

#### Terragrunt Scale

- Product type – GitOps automation platform
- Objective – automate Terragrunt workflows in CI/CD
- Website – https://terragrunt.com/terragrunt-scale
- Docs – https://docs.terragrunt.com/terragrunt-scale/overview/

### Overview

#### Terragrunt overview

Terragrunt is an IaC orchestration tool for OpenTofu and Terraform. Where OpenTofu/Terraform is the execution engine, Terragrunt is the orchestration and configuration layer to run IaC at scale. Terragrunt treats OpenTofu/Terraform modules as versioned, reusable infrastructure patterns, and deploys those modules consistently across environments.

#### Terragrunt core concepts

**Units.** A Terragrunt unit is a directory containing a `terragrunt.hcl` file. It is the smallest deployable entity in Terragrunt and is intended to be independently operable, atomic, and reproducible. This lets teams isolate infrastructure changes to smaller portions of the system instead of applying changes to a large monolithic OpenTofu/Terraform root module.

**Stacks.** A stack is a collection of related units that can be managed together. Stacks help teams deploy multiple infrastructure components with a single command, manage dependencies between units, control the blast radius of changes, and organize infrastructure into logical groups. Terragrunt supports both implicit stacks, based on directory structure, and explicit stacks, defined with `terragrunt.stack.hcl` files.

**Run Queue and dependency graph.** Terragrunt's Run Queue controls ordering and concurrency when running OpenTofu/Terraform commands across multiple units. It uses a directed acyclic graph built from dependencies between units, ensuring that dependencies run before dependent units for operations such as plan or apply, and that destroy operations occur in a safe reverse order.

**Remote state management.** Terragrunt can generate backend configuration and help manage remote state settings in a reusable way. Its `remote_state` functionality supports common backend patterns and can help bootstrap state resources such as S3 buckets and DynamoDB tables for AWS, reducing manual state setup and improving consistency.

**DRY configuration.** Terragrunt allows shared configuration to be defined once and inherited across units. Common backend settings, provider configuration, module inputs, and environment conventions can be centralized, reducing repeated code across development, staging, and production.

**Catalog and scaffold.** Terragrunt includes a catalog ecosystem with a terminal UI for browsing OpenTofu/Terraform modules, boilerplate templates and Terragrunt units/stacks. It also supports a scaffold command for generating files from these components. This supports self-service infrastructure patterns for platform teams.

#### Terragrunt Scale overview

Terragrunt Scale is the GitOps platform that automates Terragrunt workflows. All operations run inside the customer's own runners and repositories using standard GitHub Actions or GitLab CI Pipelines. Terragrunt Scale never holds direct access to cloud accounts or state files, and teams retain full control to customize their workflows as needed.

Terragrunt Scale integrates directly with native version control and CI/CD technologies. A GitHub App or GitLab machine user connects to repositories, infrastructure changes trigger plan/apply workflows in the organization's own runners, OIDC handshakes provide temporary least-privilege credentials, and Terragrunt's DAG ensures resources are created, updated, or destroyed in the correct order.

Terragrunt Scale supports GitHub, GitLab, GitHub Enterprise, and GitLab Self-Managed for version control; Terragrunt, OpenTofu, and Terraform for IaC; and AWS, GCP, Azure, and other OpenTofu/Terraform-supported platforms with custom authentication.

#### Terragrunt Scale products

**Pipelines.** A secure GitOps CI/CD pipeline built for Terragrunt. Plans run automatically when a pull or merge request is opened, and applies run on merge, all in the customer's own runners, not on Gruntwork servers. Plan summaries and logs appear directly in PR/MR comments. Pipelines executes updates through the dependency graph so adds, changes, and destroys triggers in the right order, even across environments with different access controls. It minimizes blast radius by identifying the smallest set of units affected by a change, and it uses an "apply after merge" model with OIDC-based, least-privilege, per-environment authentication.

**Drift Detection.** Scheduled and on-demand scans compare deployed cloud resources against the IaC definitions. When drift is found, Terragrunt Scale opens a PR/MR to report and remediate it, so nothing changes without review and approval. The team can review the drift, decide whether the code or the cloud state is correct, and then merge a remediation path.

**Patcher.** Automated dependency updates for OpenTofu/Terraform modules and Terragrunt units/stacks. Patcher scans Terragrunt units and stacks, and OpenTofu/Terraform module references to find current versions and available updates. It opens PRs/MRs with updated version pins and helps with breaking changes by applying patches where available or generating guidance files when manual action is required.

### Primary use cases

#### Terragrunt use cases

**Reducing Code Duplication Across Environments.** Terragrunt is especially useful when the same infrastructure pattern must be deployed across multiple environments. For example, a company might deploy a VPC, EKS cluster, database, and application stack in development, staging, and production. Terragrunt lets the organization define common configuration once, then override only the values that differ by environment.

**Managing Dependencies Between Infrastructure Components.** Infrastructure often has ordering requirements: networks before clusters, clusters before workloads, IAM before services, databases before applications. Terragrunt's dependency graph and Run Queue help automate this sequencing while still allowing safe concurrency where possible.

**Standardizing Remote State and Provider Configuration.** Remote state and provider settings are often repeated across Terraform root modules. Terragrunt allows those settings to be generated or inherited from shared configuration, which improves consistency and reduces the chance that one environment is configured differently by accident.

**Breaking Monolithic Infrastructure Into Smaller Units.** Many teams start with a large Terraform root module that manages too much infrastructure at once. Terragrunt encourages a model where infrastructure is split into smaller units, each with its own state and deployment boundary. This reduces risk as teams can plan, apply, and troubleshoot smaller parts of the system independently.

### Terragrunt Scale use cases

**GitOps Infrastructure Delivery.** Terragrunt Scale automates the GitOps workflow for infrastructure. With Pipelines, infrastructure changes in pull or merge requests trigger Terragrunt plans, while changes merged to the deploy branch trigger apply operations. This creates a predictable review process: propose infrastructure change, review plan output, merge approved change, apply through CI/CD.

**Drift Detection and Remediation.** Infrastructure drift occurs when deployed cloud resources no longer match the committed IaC. Terragrunt Scale's Drift Detection runs terragrunt plan against infrastructure units; when drift is detected, it creates a pull or merge request to track the drift and, after merge, runs terragrunt apply on the affected units to bring resources back in line with the code.

**Automated Dependency Updates.** Patcher helps keep infrastructure dependencies current. It handles non-breaking updates and also provides a workflow for breaking changes by applying patches where available or generating guidance files when manual action is required. This is especially helpful for organizations that use many reusable modules and need a repeatable process for staying current without relying entirely on manual tracking.

### Ideal customer profile

#### Terragrunt ideal users and organizations

Terragrunt is most useful for teams that already use, or plan to use, OpenTofu or Terraform at meaningful scale. Primary users are platform engineers, DevOps engineers, SREs, cloud infrastructure engineers, cloud architects, and security-minded infrastructure teams.

Companies that benefit most from Terragrunt usually have one or more of the following characteristics:

- Multiple environments
- Multiple cloud accounts, projects, subscriptions, or regions
- Many OpenTofu/Terraform modules
- Repeated infrastructure patterns
- Growing platform engineering function

#### Terragrunt Scale ideal users and organizations

Terragrunt Scale is most valuable for organizations that want to operationalize Terragrunt across teams, repositories, and cloud environments. Primary users are platform engineers, DevOps engineers, SREs, cloud infrastructure engineers, cloud architects, and security-minded infrastructure teams. Terragrunt Scale is particularly compelling once infrastructure delivery becomes a team or organizational process rather than an individual engineer's local workflow.

The companies that benefit most from Terragrunt Scale usually have one or more of the following characteristics:

- Teams already using Terragrunt
- Multiple infrastructure contributors
- Many infrastructure units or stacks
- Regulated or security-sensitive environments

### Relationship Between Terragrunt and Terragrunt Scale

Terragrunt and Terragrunt Scale are complementary.

Terragrunt remains the engine for organizing IaC across units, managing dependencies through a directed acyclic graph (DAG), and keeping configurations DRY. Terragrunt Scale adds the operational workflows most teams otherwise have to build and maintain themselves: pull-request plans, apply-on-merge, blast-radius minimization, least-privilege cloud authentication, drift detection, and automated dependency updates.
