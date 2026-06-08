---
title: Authoring Units
description: Design composable, reusable modules and turn them into Terragrunt units
slug: guides/terragrunt-101/authoring-units
sidebar:
  order: 6
---

## Learning Objectives

Upon completing this module, you will be able to:

| Objective                  | What You'll Learn                             |
| :------------------------- | :-------------------------------------------- |
| **Design modules**         | For composability and reusability             |
| **Recognize antipatterns** | Why monolithic modules create problems        |
| **Apply patterns**         | Module designs that work well with Terragrunt |
| **Build implicit stacks**  | Deploy using Terragrunt                       |

## Designing Effective Modules

A well-designed module represents the **smallest independently deployable piece of infrastructure** that makes sense for your organization.

> In Terragrunt terminology, when you wrap a module with a `terragrunt.hcl` configuration, it becomes a **unit**.

---

### Design Principles

| Principle               | Description                                                      |
| :---------------------- | :--------------------------------------------------------------- |
| **Hermetic**            | Self-contained with all necessary configuration                  |
| **Atomic**              | Changes apply as a single operation                              |
| **Appropriately sized** | Small enough for quick operations, large enough to be meaningful |

> Modules should **trend smaller** over time. It's better to create a VPC as one module, subnets as another, rather than combining everything into a monolithic "networking" module.

---

### Benefits of Smaller Modules

| Benefit                       | Impact                   |
| :---------------------------- | :----------------------- |
| **Faster plan/apply cycles**  | Quick feedback           |
| **Easier to parallelize**     | Multiple units at once   |
| **Easier to debug**           | Less code to examine     |
| **Reduced blast radius**      | Isolated failures        |
| **Better collaboration**      | Teams work independently |
| **More flexible composition** | Mix and match            |

## The Monolithic Antipattern

Before looking at good design, let's examine **what to avoid**.

---

### What Monolithic Looks Like

A monolithic networking module might create:

- VPC and internet gateway
- Public, private, and isolated subnets across multiple AZs
- NAT gateways and route tables
- VPC endpoints for AWS services

All in **one module** with 20+ input variables and conditional logic to enable/disable features.

---

#### Problems This Creates

| Problem                 | Impact                                                             |
| :---------------------- | :----------------------------------------------------------------- |
| **Slow plans**          | Every plan evaluates all resources, even for a single route change |
| **Large blast radius**  | Any change risks the entire network stack                          |
| **Team blocking**       | Only one person can work on "networking"                           |
| **Take-all-or-nothing** | Can't use just the VPC piece elsewhere                             |
| **Slow feedback loops** | Discourages small, safe changes                                    |

---

### The Composable Alternative

The same infrastructure split into **focused modules**:

| Module           | Responsibility                           | Typical Size |
| :--------------- | :--------------------------------------- | :----------- |
| **`vpc`**        | VPC, internet gateway, default resources | ~200 lines   |
| **`vpc-subnet`** | Subnets, route tables, NAT gateways      | ~120 lines   |
| **`vpc-route`**  | Individual routes                        | ~40 lines    |

> Each module does **one thing well** and exposes inputs and outputs for downstream consumption.

---

#### What This Enables

| Benefit                | Example                                   |
| :--------------------- | :---------------------------------------- |
| **Targeted plans**     | ~2 resources for a route change           |
| **Small blast radius** | Route changes don't risk VPC changes      |
| **Parallel work**      | Teams can work on different units         |
| **Mix and match**      | Different modules for different use cases |
| **Fast feedback**      | Encourages incremental improvements       |

---

### The Difference in Practice

| Aspect                       | Monolithic           | Composable               |
| :--------------------------- | :------------------- | :----------------------- |
| **Plan time** (route change) | All 30+ resources    | ~2 resources             |
| **Blast radius**             | Entire network stack | Isolated to changed unit |
| **Team collaboration**       | Blocked—one owner    | Parallel work possible   |
| **Reusability**              | Take all or nothing  | Mix and match            |

> The composable approach requires more files, but this is a **feature, not a bug**. Each file represents a clear boundary with explicit dependencies.

## Module Design Patterns

When authoring modules that Terragrunt will consume, a few patterns make them **easier to work with**.

> The patterns below apply to **module code**, which lives in `.tf` files (OpenTofu/Terraform). That's distinct from the `.hcl` files Terragrunt uses to wrap and configure those modules — the variables and outputs shown here are plain `.tf`.

---

### Expose Rich Outputs

Downstream units need to reference your outputs. Export **structured objects** rather than individual values, and define **one output per resource class**:

```hcl
# outputs.tf

output "vpc" {
  value = {
    id              = aws_vpc.vpc.id
    arn             = aws_vpc.vpc.arn
    ipv4_cidr_block = aws_vpc.vpc.cidr_block
  }
}

output "route" {
  value = {
    id                     = aws_route.route.id
    destination_cidr_block = aws_route.route.destination_cidr_block
    gateway_id             = aws_route.route.gateway_id
  }
}
```

> When a module manages more than one resource type, give each type its own grouped output. Dependent units then reference them clearly: `dependency.network.outputs.vpc.id` or `dependency.network.outputs.route.id`.

Why structure outputs this way?

- **Grouped by resource** - one output per resource keeps it obvious which value comes from where, even as the module grows
- **Mirrors the original resource structure** - each output's shape resembles the resource it wraps, making it intuitive to use
- **Groups related attributes together** - cross-dependent values stay together logically
- **Simplifies expansion** - adding new attributes or implementing loops/conditionals is straightforward without breaking the interface

---

### Group Related Inputs

Use **object variables** to group related settings:

```hcl
# variables.tf

variable "vpc_settings" {
  type = object({
    cidr_block           = optional(string)
    enable_dns_hostnames = optional(bool, true)
    enable_dns_support   = optional(bool, true)
  })
}
```

> This keeps the module interface **clean** and makes it obvious which settings belong together.

---

### Provide Sensible Defaults

Modules should work with **minimal configuration**.

Use `optional()` with defaults so users only specify what they need to change.

---

### Accept Tags Consistently

Allow tags to **flow through** from Terragrunt's include hierarchy:

```hcl
# variables.tf

variable "tags" {
  type    = map(string)
  default = {}
}
```

---

### Pattern Summary

| Pattern               | Benefit                                    |
| :-------------------- | :----------------------------------------- |
| **Rich outputs**      | Clear, structured access to values         |
| **Grouped inputs**    | Clean interface, related settings together |
| **Sensible defaults** | Minimal configuration needed               |
| **Consistent tags**   | Inherit from include hierarchy             |

## Building an Implicit Stack

An **implicit stack** is the traditional way of organizing Terragrunt configurations.

The directory structure ***is*** the stack—Terragrunt discovers units by walking the filesystem and builds a dependency graph from your `dependency` blocks.

---

### The Environment-Based Structure

A typical implicit stack follows an environment-based hierarchy:

```text
live/
├── dev/
│   └── us-east-1/
│       └── network/
│           ├── vpc/
│           │   └── terragrunt.hcl
│           ├── subnets/
│           │   └── terragrunt.hcl
│           ├── routes-public/
│           │   └── terragrunt.hcl
│           ├── routes-private-az1/
│           │   └── terragrunt.hcl
│           └── routes-private-az2/
│               └── terragrunt.hcl
└── root.hcl
```

---

#### What the Structure Encodes

| Level           | Example          | Meaning                                    |
| :-------------- | :--------------- | :----------------------------------------- |
| **Environment** | `dev`            | Which deployment environment               |
| **Region**      | `us-east-1`      | Which AWS region                           |
| **Component**   | `network`        | Logical grouping of related infrastructure |
| **Unit**        | `vpc`, `subnets` | Individual deployable pieces               |

---

### Running Implicit Stacks

Deploy all units in a directory with **`terragrunt run --all`**:

```bash
cd live/dev/us-east-1/network
terragrunt run --all plan    # Plan all units
terragrunt run --all apply   # Apply all units in dependency order
```

Terragrunt automatically:

| Step  | Action                                                                 |
| :---: | :--------------------------------------------------------------------- |
| **1** | Discovers all `terragrunt.hcl` files in the directory tree             |
| **2** | Builds a dependency graph from `dependency` blocks                     |
| **3** | Executes operations in the correct order *(or in parallel where safe)* |

---

### Advantages of Implicit Stacks

| Advantage                  | Description                                           |
| :------------------------- | :---------------------------------------------------- |
| **Intuitive**              | The filesystem *is* the stack definition              |
| **No extra configuration** | Works with just `terragrunt.hcl` files                |
| **Flexible**               | Run against any directory level                       |
| **Familiar**               | Works like traditional Terraform directory structures |

---

### Limitations of Implicit Stacks

| Limitation             | Description                                        |
| :--------------------- | :------------------------------------------------- |
| **Duplication**        | Reusing a stack means copying directories          |
| **No versioning**      | Can't version the stack as a single artifact       |
| **Environment sprawl** | Adding environments requires duplicating structure |
| **Hardcoded values**   | Configuration lives in individual files            |

> These limitations motivate **explicit stacks**, which you'll learn about in [**Module 7**](/guides/terragrunt-101/composing-stacks/).
