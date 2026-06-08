---
title: Composing Stacks
description: From implicit to explicit stacks with terragrunt.stack.hcl — defining, generating, and composing stacks
slug: guides/terragrunt-101/composing-stacks
sidebar:
  order: 7
---

## Learning Objectives

Upon completing this module, you will be able to:

| Objective                     | What You'll Learn                |
| :---------------------------- | :------------------------------- |
| **Understand the difference** | Implicit vs. explicit stacks     |
| **Define explicit stacks**    | Using `terragrunt.stack.hcl`     |
| **Use stack commands**        | Generate, plan, and apply stacks |
| **Pass values**               | To units within a stack          |
| **Compose stacks**            | From units and nested stacks     |

## From Implicit to Explicit Stacks

Throughout this course, you've been working with **implicit stacks**.

When you organize units in a directory structure and run `terragrunt run --all`, Terragrunt treats that directory as a stack. It discovers units by walking the filesystem and builds a dependency graph from your `dependency` blocks.

```text
dev/
├── vpc/
│   └── terragrunt.hcl
├── security-group/
│   └── terragrunt.hcl
└── web-server/
    └── terragrunt.hcl
```

---

### Advantages of Implicit Stacks

| Advantage                  | Description                              |
| :------------------------- | :--------------------------------------- |
| **Intuitive**              | The filesystem *is* the stack definition |
| **No extra configuration** | Works out of the box                     |
| **Natural fit**            | Works with existing Terragrunt projects  |

---

### Limitations of Implicit Stacks

| Limitation                     | Impact                                                            |
| :----------------------------- | :---------------------------------------------------------------- |
| **Directory-based definition** | Stack definition lives in structure, not code                     |
| **Copying required**           | Reusing a stack means copying directories                         |
| **No versioning**              | Can't version a stack and its inputs as a single artifact         |
| **Duplication**                | Deploying to multiple environments requires duplicating structure |

---

### The Solution: Explicit Stacks

**Explicit stacks** solve these problems by defining stacks programmatically in a `terragrunt.stack.hcl` file.

## The Stack Definition File

An explicit stack is defined in a file named **`terragrunt.stack.hcl`**:

```hcl
# terragrunt.stack.hcl

unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "security_group" {
  source = "../units/security-group"
  path   = "security-group"
}

unit "web_server" {
  source = "../units/web-server"
  path   = "web-server"
}
```

When you run `terragrunt stack generate`, Terragrunt creates the directory structure with `terragrunt.hcl` files in each path.

> The stack definition becomes the **source of truth**. Regenerating produces the same structure every time.

---

### Unit Block Attributes

These are the most important attributes to remember when working with [`unit`](https://docs.terragrunt.com/reference/hcl/blocks/#unit) blocks.

| Attribute    | Required | Description                                            |
| :----------- | :------- | :----------------------------------------------------- |
| **`source`** | Yes      | Where to fetch the unit configuration from             |
| **`path`**   | Yes      | Local directory path for the generated unit            |
| **`values`** | No       | Values to pass to the unit *(accessed via `values.*`)* |

> The `source` attribute supports the same formats as regular units (local paths, Git SSH, Git HTTPS). Using Git sources with **version tags** enables treating your stack as a **versioned artifact**.

## Passing Values to Units

Both units and stacks can receive **environment-specific configuration** via values.

The `values` attribute passes data into units:

```hcl
# terragrunt.stack.hcl

unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"

  values = {
    name_prefix = "production"
    cidr_block  = "10.0.0.0/16"
  }
}
```

---

### Accessing Values in Units

Inside the unit's `terragrunt.hcl`, access these values via **`values.*`**:

```hcl
# units/vpc/terragrunt.hcl

terraform {
  source = "../../modules/vpc"
}

inputs = {
  name_prefix  = values.name_prefix
  vpc_settings = {
    cidr_block = values.cidr_block
  }
}
```

---

### Conditional Logic with Values

Values enable the same definition to **adapt to different contexts**:

```hcl
# units/vpc/terragrunt.hcl
locals {
  # Default to dev settings if not in a stack context
  name_prefix = try(values.name_prefix, "dev")
  cidr_block  = try(values.cidr_block, "10.0.0.0/16")
}

inputs = {
  name_prefix  = local.name_prefix
  vpc_settings = {
    cidr_block = local.cidr_block
  }
}
```

> Using `try()` allows the unit to define sensible defaults for values that don't _have_ to be defined explicitly for generation.

---

### Values Flow Through Stacks

The same pattern works at the stack level. A stack definition can accept values and pass them to its units:

```hcl
# stacks/vpc/terragrunt.stack.hcl

locals {
  # Accept values from parent, with defaults
  environment = try(values.environment, "dev")
  cidr_block  = try(values.cidr_block, "10.0.0.0/16")
}

unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"

  values = {
    name_prefix = local.environment
    cidr_block  = local.cidr_block
  }
}
```

This creates a **single definition** that can be instantiated with different values for each environment.

## Working with Stack Commands

Explicit stacks introduce new commands under **`terragrunt stack`**:

| Command                            | Description                              |
| :--------------------------------- | :--------------------------------------- |
| **`terragrunt stack generate`**    | Generate units from the stack definition |
| **`terragrunt stack run plan`**    | Plan all units in the stack              |
| **`terragrunt stack run apply`**   | Apply all units in the stack             |
| **`terragrunt stack run destroy`** | Destroy all units in reverse order       |
| **`terragrunt stack output`**  | Show consolidated outputs from all units              |

---

### The Generate Step

Before operating on an explicit stack, you can explicitly **generate it**:


Stacks are generated automatically by default for all commands that interact with stacks.

```bash
cd my-stack/
terragrunt stack generate
```

This creates the directory structure defined in your `terragrunt.stack.hcl`. Each unit gets its own directory with a `terragrunt.hcl` file.

When a stack includes a `stack` block (referencing another stack), the referenced stack's units are also generated.

---

### Generated File Management

By default, `terragrunt stack generate` creates generated units in their `path` directories. A common pattern is to:

1. Keep stack definitions in version control
2. Add generated files to `.gitignore`
3. Generate fresh in CI/CD before applying

> Treat the `terragrunt.stack.hcl` as the source of truth; generated files are ephemeral.

## Nested Stacks

Stacks can include **other stacks**, enabling composition at a higher level:

```hcl
# terragrunt.stack.hcl

unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

stack "monitoring" {
  source = "../stacks/monitoring"
  path   = "monitoring"
}

stack "logging" {
  source = "../stacks/logging"
  path   = "logging"
}
```

> The `stack` block works similarly to `unit`, but references another `terragrunt.stack.hcl` file instead of a `terragrunt.hcl` file.

---

### Stack Block Attributes

These are the most important attributes to remember when working with [`stack`](https://docs.terragrunt.com/reference/hcl/blocks/#stack) blocks.

| Attribute    | Required | Description                               |
| :----------- | :------- | :---------------------------------------- |
| **`source`** | Yes      | Path to another stack definition          |
| **`path`**   | Yes      | Local directory path for the nested stack |
| **`values`** | No       | Values to pass to the nested stack        |

---

### Building a Stack Catalog

Nested stacks enable building **reusable infrastructure blueprints**:

```text
catalog/
├── stacks/
│   ├── eks-cluster/
│   │   └── terragrunt.stack.hcl
│   ├── monitoring/
│   │   └── terragrunt.stack.hcl
│   └── full-environment/
│       └── terragrunt.stack.hcl  # Composes eks-cluster + monitoring
└── units/
    ├── vpc/
    ├── eks/
    └── prometheus/
```

| Level                | Purpose                              |
| :------------------- | :----------------------------------- |
| **Units**            | Individual infrastructure components |
| **Stacks**           | Composed collections of units        |
| **Full environment** | Composed collections of stacks       |

## Environment Pattern with Explicit Stacks

A powerful pattern is defining a **single stack blueprint** and deploying it **multiple times** with different configurations:

```text
project/
├── stacks/
│   └── three-tier-vpc/
│       └── terragrunt.stack.hcl        # The blueprint (accepts values)
├── units/
│   └── ...                             # Reusable unit templates
└── infrastructure-live/
    └── dev/
        └── us-east-1/
            ├── vpc-dev1/
            │   └── terragrunt.stack.hcl    # Instantiates blueprint for first VPC
            └── vpc-dev2/
                └── terragrunt.stack.hcl    # Instantiates blueprint for second VPC
```

---

### The Blueprint Stack

The central stack definition accepts values and passes them to its units:

```hcl
# stacks/three-tier-vpc/terragrunt.stack.hcl
locals {
  # Accept values from parent, with defaults for testing
  environment = try(values.environment, "dev")
  vpc_cidr    = try(values.vpc_cidr, "10.0.0.0/16")
  region      = try(values.region, "us-east-1")
  azs         = try(values.azs, ["us-east-1a", "us-east-1b"])

  # Units are outside infrastructure-live, so we navigate up from repo root
  units_dir = "${get_repo_root()}/../units"
}

unit "vpc" {
  source = "${local.units_dir}/vpc"
  path   = "vpc"
  values = {
    environment = local.environment
    vpc_cidr    = local.vpc_cidr
  }
}

unit "vpc_subnet" {
  source = "${local.units_dir}/vpc-subnet"
  path   = "vpc-subnet"
  values = {
    environment = local.environment
    region      = local.region
    azs         = local.azs
  }
}
# ... more units (routes, etc.)
```

---

### Stack Instantiation Files

Each instantiation creates a small file that references the blueprint and passes values:

#### First VPC (vpc-dev1)

```hcl
# infrastructure-live/dev/us-east-1/vpc-dev1/terragrunt.stack.hcl
stack "network" {
  source = "${get_repo_root()}/../stacks/three-tier-vpc"
  path   = "."
  values = {
    environment = "dev1"
    vpc_cidr    = "10.0.0.0/16"
    region      = "us-east-1"
    azs         = ["us-east-1a", "us-east-1b"]
  }
}
```

#### Second VPC (vpc-dev2)

```hcl
# infrastructure-live/dev/us-east-1/vpc-dev2/terragrunt.stack.hcl
stack "network" {
  source = "${get_repo_root()}/../stacks/three-tier-vpc"
  path   = "."
  values = {
    environment = "dev2"
    vpc_cidr    = "10.1.0.0/16"
    region      = "us-east-1"
    azs         = ["us-east-1a", "us-east-1b"]
  }
}
```

> These files are simple. They just specify the `source` and instance-specific `values`.

---

### Benefits of This Pattern

| Benefit                    | Description                               |
| :------------------------- | :---------------------------------------- |
| **Single definition**      | One place defines what the stack contains |
| **Isolated configuration** | Instance-specific values in simple files  |
| **Consistency**            | All instances use the same blueprint      |
| **Easy scaling**           | Adding new instances is simple            |
