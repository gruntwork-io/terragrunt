---
title: First Principles and Terminology
description: The problems Terragrunt solves and the core vocabulary you'll use throughout the course
slug: guides/terragrunt-101/first-principles-terminology
sidebar:
  order: 2
---

## Learning Objectives

After completing this module, you'll be able to:

| Objective                                   | Key Concepts                          |
| :------------------------------------------ | :------------------------------------ |
| **Explain** what problems Terragrunt solves | IaC Orchestration, DRY IaC            |
| **Define** core terminology                 | Units, stacks, includes, dependencies |
| **Describe** orchestration                  | How Terragrunt coordinates runs       |
| **Explain** blast radius                    | Why it matters for safe deployments   |

## Why Terragrunt Exists

Terragrunt is an **orchestration layer** for OpenTofu / Terraform.

It **doesn't replace** them. Instead, it **coordinates** how and when your infrastructure gets deployed.

---

### The Five Problems Terragrunt Solves

#### 1. Managing Dependencies

Terragrunt builds a **Directed Acyclic Graph (DAG)** to:

- Coordinate deployments across multiple OpenTofu/Terraform root modules with their own state files
- Run units in the **correct dependency order**

#### 2. Eliminating Repetition (DRY)

Write shared configuration **once**, then **reuse** them through includes and stack definitions.

#### 3. Segmenting State for Smaller Blast Radius

Split infrastructure into **independent units** with separate state files to:

- Isolate changes
- Reduce risk
- Limit the impact of failures

#### 4. Automating Remote State Backend

Terragrunt **automatically creates and configures** backend resources, including:

- S3 buckets
- GCS buckets
- Azure blob stores [(coming soon)](/reference/experiments/active/#azure-backend)
- DynamoDB tables

This eliminates manual backend bootstrapping before using backends for OpenTofu/Terraform state.

#### 5. Generating Providers and Backends Dynamically

Terragrunt generates provider and backend configuration **dynamically** based on your environment. This removes the need to hardcode these blocks in every root module.

---

### How It All Fits Together

You write OpenTofu / Terraform modules as **generic patterns**.

Terragrunt then:

- **Instantiates** those patterns with specific configurations
- **Wires** them together by passing outputs between units
- **Runs** them in the correct dependency order

:::tip
Modules define ***what*** to build.
Terragrunt defines ***how*** and ***when***.
:::

## Core Terminology

### Units

A [**unit**](/getting-started/terminology/#unit) is a single instance of infrastructure with its own state file.

Terragrunt detects units by looking for directories containing `terragrunt.hcl` files. Each unit is **self-contained** and represents the **smallest unit of infrastructure you want to deploy** with Terragrunt.

:::tip
> Common examples of units include a single VPC, a single database instance, or an application service.
:::

---

### Stacks

A [**stack**](/getting-started/terminology/#stack) is a collection of units managed together.

Terragrunt supports **two types**:

| Type                | Description                                                                  |
| :------------------ | :--------------------------------------------------------------------------- |
| **Implicit stacks** | Created implicitly through directory organization                            |
| **Explicit stacks** | Defined in `terragrunt.stack.hcl` files that generate units programmatically |

---

### Dependencies

Terragrunt provides **two mechanisms** for defining [dependency](/getting-started/terminology/#dependency) relationships between units:

| Block                                                       | Purpose                             | When to Use                                              |
| :---------------------------------------------------------- | :---------------------------------- | :------------------------------------------------------- |
| [**`dependency`**](/reference/hcl/blocks/#dependency)       | Retrieves outputs from another unit | When you need **outputs** from one unit in another       |
| [**`dependencies`**](/reference/hcl/blocks/#dependencies)   | Defines ordering relationships      | When units must run in order but **don't exchange data** |

Terragrunt reads these declarations to build the directed acyclic graph (DAG) which is used to determine the order in which it executes OpenTofu/Terraform.

In practice, you'll typically only need the `dependency` block. It handles ordering on its own, and most of the time you're defining a dependency relationship precisely because one unit needs to read outputs from another. Reach for `dependencies` only when units must run in a specific order but **don't** exchange any outputs.

---

### Includes

**Includes** let one `terragrunt.hcl` file merge in another file as partial configuration.

This is how you **share common settings** and keep your configs **DRY**.

Support for this in stacks is coming soon — see the [`stack-dependencies`](/reference/experiments/active/#stack-dependencies) experiment for more information.

---

### Directed Acyclic Graph (DAG)

A [**DAG**](/getting-started/terminology/#directed-acyclic-graph-dag) is how Terragrunt determines the order in which units run. Let's break down the term:

| Word         | Meaning                        | Example                                       |
| :----------- | :----------------------------- | :-------------------------------------------- |
| **Directed** | Relationships flow one way     | A → B means "A depends on B"                  |
| **Acyclic**  | No circular dependencies       | You can't have A → B → C → back to A          |
| **Graph**    | A structure of connected nodes | Similar to a project dependency chart         |

Because Terragrunt builds and leverages a DAG, it can reliably schedule concurrent runs for units that can safely run concurrently (as they don't depend on the results of other pending runs). It can also reliably schedule the runs in the right order, as dependencies will always run before dependents for plan and apply operations, and dependents will always run before dependencies for destroy operations.

#### Think of it like doing the chores

Imagine you're doing the dishes and the laundry at the same time. Each has its own ordered sequence of steps:

```text
Scrape dishes → Load dishwasher → Wash → Dry → Put away

Load washer → Wash → Load dryer → Dry → Fold → Put away
```

Within each chore the order is fixed — you can't dry the dishes before they've been washed, and you can't fold clothes before they're dry. But the two chores don't depend on each other, so they can happen in parallel: while the dishwasher is running, you can load the washing machine.

Terragrunt works the same way. If your app server needs a VPC to exist first, Terragrunt ensures the VPC is created before provisioning the app server. When there is no such dependency relationship blocking provisioning, Terragrunt runs units concurrently.

#### How Terragrunt uses the DAG

When you run `terragrunt run --all apply`, Terragrunt:

1. **Discovers** all units in the current directory tree
2. **Builds** a [Run Queue](https://docs.terragrunt.com/getting-started/terminology/#run-queue), respecting the DAG from declared dependencies
3. **Runs** units in topological order in the [Runner Pool](https://docs.terragrunt.com/getting-started/terminology/#runner-pool), parallelizing where possible

When you **destroy**, the order reverses—dependents are removed before their dependencies.

## The Blast Radius Principle

Your **current working directory** defines your blast radius.

As you navigate the filesystem you are changing what infrastructure you can affect.

:::tip
> This design encourages **smaller units** over monolithic configurations.
> **Smaller** is quicker, easier, and safer.
:::

---

### Benefits of a Smaller Blast Radius

| Benefit                    | Impact                         |
| :------------------------- | :----------------------------- |
| **Faster** plan and apply  | Less waiting, quicker feedback |
| **Less risk** from changes | Isolated failures              |
| **Easier debugging**       | Fewer variables to consider    |
| **Better collaboration**   | Teams can work independently   |

---

### Why Small Units Matter

Orchestration works best with **small, focused units**.

| Unit Size         | Apply Time  | Complexity           | Recovery           |
| :---------------- | :---------- | :------------------- | :----------------- |
| Small unit        | ~2 minutes  | Easy to reason about | Quick recovery     |
| Monolithic config | ~45 minutes | Hard to understand   | Difficult recovery |

When something goes wrong, **small units are easier to fix**.

## How Terragrunt Runs OpenTofu / Terraform

When you run `terragrunt apply`, here's what happens:

| Step  | Action                                          |
| :---: | :---------------------------------------------- |
| **1** | Downloads module sources into a cache directory |
| **2** | Copies local files to the cache directory       |
| **3** | Generates backend and provider configurations   |
| **4** | Runs the OpenTofu / Terraform command           |

*We'll explore this process in detail in [Module 3](/guides/terragrunt-101/getting-started/).*

---

### Using Terragrunt

#### Drop-in Replacement

Terragrunt works as a **drop-in replacement**. Swap `tofu` or `terraform` for `terragrunt`:

```bash
# Instead of:
tofu plan
tofu apply

# Use:
terragrunt plan
terragrunt apply
```

---

#### Multi-Unit Operations

For multi-unit operations, use **`run --all`**:

```bash
# Plan all units in the current directory tree
terragrunt run --all plan

# Apply in dependency order
terragrunt run --all apply
```
