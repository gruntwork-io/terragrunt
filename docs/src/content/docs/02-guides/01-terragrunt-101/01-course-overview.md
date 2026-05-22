---
title: Course Overview
description: Welcome and orientation for the Terragrunt 101 self-paced course
slug: guides/terragrunt-101
sidebar:
  order: 1
prev: false
---

## Welcome to Terragrunt 101

This is a **self-paced course** for engineers who already use OpenTofu or Terraform and want to level up their Infrastructure as Code (IaC) with Terragrunt.

---

### Why Terragrunt?

Terragrunt helps you **organize**, **reuse**, and **orchestrate** infrastructure at scale, without the copy/paste and repetition that OpenTofu or Terraform alone often require.

**What Terragrunt does really well today:**

- **Stacks**: Split infrastructure into small, deployable [**units**](https://docs.terragrunt.com/getting-started/terminology/#unit) and combine them into [**stacks**](https://docs.terragrunt.com/getting-started/terminology/#stack). Define a stack once (e.g., a VPC plus subnets plus security groups) and instantiate it across environments or accounts with different values. No duplicated directory trees.
- **Keep things DRY**: Share backend config, CLI arguments, and inputs across all your units via a single root config. The OpenTofu/Terraform `backend` block doesn't support variables; Terragrunt does, so you write configuration once and reuse it everywhere.
- **Catalog**: Scaffold new units quickly with `terragrunt catalog` to enable infrastructure self-service using Infrastructure as Code.
- **Orchestration**: Run multiple units in the right order. Terragrunt understands dependencies between modules and manages multiple state files so you can plan, apply and destroy in dependency order without manual scripting.

If you've outgrown ad-hoc copy/paste, repeated backend blocks, or custom scripts to chain OpenTofu/Terraform runs, this course will show you how Terragrunt addresses that. You'll learn the concepts (units, stacks, root config), the HCL blocks and functions that make config reusable, and how to author and compose stacks so your IaC stays maintainable as it grows.

---

#### A bit of history

Terragrunt came out of Gruntwork in July 2016. The team was running Terraform for dozens of clients and kept hitting the same limits: Terraform had no remote state locking, so if two people ran apply at once they could overwrite each other's state. The docs said locking wasn't supported and pointed you at Atlas (the paid product that became Terraform Enterprise). There was no way to express remote state as code, so every module got its backend config copy-pasted or hand-wired, which led to mistakes and duplicate resources.

Terragrunt started as a thin wrapper that managed remote state for you and provided locking via DynamoDB. Over the next few years, Terraform adopted much of that (e.g., DynamoDB-style state locking landed in 0.9 in 2017). As those gaps closed, different pains became obvious: the `backend` block still didn't support variables, so teams kept copy-pasting. Common CLI flags had to be repeated on every run. Promoting the same module from dev to staging to prod meant copying whole directory trees. Coordinating multiple state files in the right order meant custom scripting. Terragrunt evolved to address those problems and kept evolving into what it is today: a tool built around keeping config DRY and helping teams split infrastructure into units and orchestrate them as stacks.

## What You'll Learn

Terragrunt helps you **organize** and **reuse** your infrastructure code.

---

### Getting Started

You'll start with the basics in [**Module 2: First Principles**](/guides/terragrunt-101/first-principles-terminology/), where you'll learn:
- What are **units** and **stacks**?
- How does Terragrunt run your OpenTofu or Terraform code?

Then in [**Module 3: Getting Started**](/guides/terragrunt-101/getting-started/), you'll install Terragrunt, learn the commands, and set up your first project.

---

### Building Skills

[**Module 4: HCL Blocks**](/guides/terragrunt-101/blocks/) introduces the `terraform`, `include`, and `dependency` blocks that power Terragrunt configurations.

[**Module 5: Functions**](/guides/terragrunt-101/functions/) covers path helpers, environment lookups, and shell execution—tools that help you share configuration across environments *without copying and pasting*.

In [**Module 6: Authoring Units**](/guides/terragrunt-101/authoring-units/), you'll write your own reusable units using inputs, generate blocks, and include patterns.

[**Module 7: Composing Stacks**](/guides/terragrunt-101/composing-stacks/) teaches you to combine units into stacks with dependencies—both implicit and explicit.

---

### Advanced Topics

[**Module 8: Advanced Patterns**](/guides/terragrunt-101/advanced-patterns/) brings it all together with:
- Dynamic credentials with `auth-provider-command`
- Run-queue strategies and filtering
- Error handling (retries and ignoring expected errors)

## Resources

### Terragrunt Documentation

For command references and configuration details:

| Resource | Description |
|:---------|:------------|
| [**Full documentation**](/) | Complete reference |
| [**Configuration blocks**](/reference/hcl/blocks/) | HCL block reference |
| [**Built-in functions**](/reference/hcl/functions/) | Function reference |
| [**CLI reference**](/reference/cli/) | Command-line options |
| [**Join us on Discord**](/community/invite) | Ask questions and chat with the Terragrunt community |