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
| **Explain** what problems Terragrunt solves | DRY IaC, IaC Orchestration            |
| **Define** core terminology                 | Units, stacks, includes, dependencies |
| **Describe** orchestration                  | How Terragrunt coordinates execution  |
| **Explain** blast radius                    | Why it matters for safe deployments   |

## Why Terragrunt Exists

Terragrunt is an **orchestration layer** for OpenTofu / Terraform.

It **doesn't replace** them. Instead, it **coordinates** how and when your infrastructure gets deployed.

---

### The Five Problems Terragrunt Solves

#### 1. Managing Dependencies

Terragrunt builds a **Directed Acyclic Graph (DAG)** to:

- Coordinate deployments across multiple state files
- Execute units in the **correct dependency order**

#### 2. Eliminating Repetition (DRY)

Write shared configuration **once** in parent files, then **inherit** it everywhere through includes and stack definitions.

#### 3. Segmenting State for Smaller Blast Radius

Split infrastructure into **independent units** with separate state files to:

- Isolate changes
- Reduce risk
- Limit the impact of failures

#### 4. Automating Remote State Backend

Terragrunt **automatically creates and configures**:

- S3 buckets
- DynamoDB tables

This eliminates manual backend setup.

#### 5. Generating Providers and Backends Dynamically

Terragrunt generates provider and backend configuration **at runtime** based on your environment—no need to hardcode these blocks in every module.

---

### How It All Fits Together

You write OpenTofu / Terraform modules as **generic patterns**.

Terragrunt then:

- **Instantiates** those patterns with specific configurations
- **Wires** them together by passing outputs between units
- **Executes** them in the correct dependency order

> Modules define ***what*** to build.
> Terragrunt defines ***how*** and ***when***.

## Core Terminology

### Units

A **unit** is a single instance of infrastructure with its own state file.

Terragrunt detects units by looking for `terragrunt.hcl` files. Each unit is **self-contained** and represents the **smallest thing you can deploy**.

> Common examples of units include a single VPC, a single database instance, or an application service.

---

### Stacks

A **stack** is a collection of units managed together.

Terragrunt supports **two types**:

| Type                | Description                                                                  |
| :------------------ | :--------------------------------------------------------------------------- |
| **Implicit stacks** | Created through directory organization *(traditional approach)*              |
| **Explicit stacks** | Defined in `terragrunt.stack.hcl` files that generate units programmatically |

---

### Dependencies

Terragrunt provides **two mechanisms** for defining relationships between units:

| Block              | Purpose                             | When to Use                                              |
| :----------------- | :---------------------------------- | :------------------------------------------------------- |
| **`dependency`**   | Retrieves outputs from another unit | When you need **data** from one unit in another          |
| **`dependencies`** | Defines ordering relationships      | When units must run in order but **don't exchange data** |

Terragrunt reads these declarations to build the directed acyclic graph (DAG) which is used to determine the order in which it executes OpenTofu/Terraform.

---

### Includes

**Includes** let one `terragrunt.hcl` file pull in another file as partial configuration.

This is how you **share common settings** and keep your configs **DRY**.

---

### Directed Acyclic Graph (DAG)

A **DAG** is how Terragrunt determines execution order. Let's break down the term:

| Word         | Meaning                        | Example                                       |
| :----------- | :----------------------------- | :-------------------------------------------- |
| **Directed** | Relationships flow one way     | A → B means "A must complete before B starts" |
| **Acyclic**  | No circular dependencies       | You can't have A → B → C → back to A          |
| **Graph**    | A structure of connected nodes | Similar to a project dependency chart         |

Because Terragrunt explicitly builds a DAG, it can easily calculate independent "paths" (without mutual dependencies) that can safely run in parallel to speed up overall execution.

#### Think of it like a build pipeline

In a CI/CD pipeline, you can't deploy before tests pass, and tests can't run before the code compiles:

```text
Compile → Test → Deploy
```

Independent stages can run in parallel, but dependent stages must wait.

Terragrunt works the same way. If your app server needs a VPC to exist first, Terragrunt ensures the VPC is created before provisioning the app server—and can parallelize units that have no dependencies on each other.

#### How Terragrunt uses the DAG

When you run `terragrunt run --all apply`, Terragrunt:

1. **Scans** all units in the current directory tree
2. **Builds** a DAG from declared dependencies
3. **Executes** units in topological order, parallelizing where possible

When you **destroy**, the order reverses—dependents are removed before their dependencies.

## The Blast Radius Principle

Your **current working directory** defines your blast radius.

As you navigate the filesystem you are changing what infrastructure you can affect.

> This design encourages **smaller units** over monolithic configurations.
> **Smaller** is quicker, easier, and safer.

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
