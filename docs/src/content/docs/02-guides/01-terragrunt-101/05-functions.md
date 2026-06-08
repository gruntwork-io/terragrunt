---
title: Terragrunt Functions
description: Built-in functions for dynamic configuration — path helpers, environment lookups, configuration readers, and shell execution
slug: guides/terragrunt-101/functions
sidebar:
  order: 5
---

## Introduction

This module covers the functions that make your Terragrunt configurations **dynamic**:

| Category                  | Purpose                         |
| :------------------------ | :------------------------------ |
| **Path helpers**          | Figure out where things are     |
| **Environment lookups**   | Read environment variables      |
| **Configuration readers** | Parse other Terragrunt files    |
| **Shell execution**       | Run commands and capture output |

## Built-in HCL Support

Terragrunt configurations are written in **HCL** (HashiCorp Configuration Language).

You have access to all the standard HCL features:

- **Conditionals**
- **Loops**
- **Expressions**

You also have full access to **OpenTofu's built-in functions** for string manipulation, collection operations, encoding/decoding, and more.

:::tip
For the complete reference, see the [OpenTofu Functions documentation](https://opentofu.org/docs/language/functions/).
:::

---

### Example: Standard HCL Features

```hcl
# terragrunt.hcl

locals {
  # String functions
  env_upper = upper(local.environment)

  # Collection functions
  all_tags = merge(local.common_tags, local.extra_tags)

  # Conditionals
  instance_type = local.environment == "prod" ? "m5.large" : "t3.micro"

  # Encoding/decoding
  config = yamldecode(file("${get_terragrunt_dir()}/config.yaml"))
}
```

---

### Terragrunt-Specific Functions

The following sections cover functions that are **unique to Terragrunt**—things like path resolution, configuration reading, and shell execution that you won't find in OpenTofu.

## Path Functions

These functions figure out **where things are** so you don't have to hardcode paths.

---

### find_in_parent_folders()

**Walks up** the directory tree looking for a file. Returns the absolute path to the first match.

```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}
```

Pass a **second argument** as a fallback if the file might not exist:

```hcl
include "env" {
  path = find_in_parent_folders("env.hcl", "default-env.hcl")
}
```

---

### path_relative_to_include()

Returns the path from the current unit to the included file.

This is how you generate **unique state keys automatically**.

```hcl
# In root.hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "my-tofu-state"
    key    = "${path_relative_to_include()}/tofu.tfstate"
    region = "us-east-1"
  }
}
```

| Unit Location                       | State Key                         |
| :---------------------------------- | :-------------------------------- |
| `prod/us-east-1/vpc/terragrunt.hcl` | `prod/us-east-1/vpc/tofu.tfstate` |

---

### get_terragrunt_dir()

Returns the **absolute path** to the directory containing the current `terragrunt.hcl`.

```hcl
terraform {
  source = "${get_terragrunt_dir()}/../../../modules/vpc"
}

inputs = {
  config_file = "${get_terragrunt_dir()}/config.json"
}
```

---

### get_repo_root()

Returns the **absolute path** to the Git repository root.

```hcl
locals {
  common_vars = yamldecode(file("${get_repo_root()}/common-vars.yaml"))
}

terraform {
  source = "${get_repo_root()}/modules/vpc"
}
```

---

### get_path_from_repo_root()

Returns your location **relative to the repo root**.

Handy for extracting context from your directory structure.

```hcl
# For a unit at prod/us-east-1/vpc/terragrunt.hcl
locals {
  path_parts = split("/", get_path_from_repo_root())
  # path_parts = ["prod", "us-east-1", "vpc"]

  aws_region = local.path_parts[1]  # "us-east-1"
}

inputs = {
  region = local.aws_region
}
```

---

### Quick Reference

| Function                         | Returns                                             |
| :------------------------------- | :-------------------------------------------------- |
| **`find_in_parent_folders()`**   | Absolute path to first matching file upward         |
| **`path_relative_to_include()`** | Relative path from included file to current         |
| **`get_terragrunt_dir()`**       | Absolute path to current `terragrunt.hcl` directory |
| **`get_repo_root()`**            | Absolute path to Git repository root                |
| **`get_path_from_repo_root()`**  | Relative path from repo root to current directory   |

> See the [official documentation](/reference/hcl/functions/) for the complete list of path functions.

## Environment and Configuration

### get_env()

Reads **environment variables**. The second argument is a default if the variable isn't set.

```hcl
locals {
  region      = get_env("AWS_REGION", "us-east-1")
  environment = get_env("ENVIRONMENT", "dev")
}
```

---

### read_terragrunt_config()

Parses another Terragrunt file and returns its contents as a **map**.

```hcl
locals {
  account_vars = read_terragrunt_config(find_in_parent_folders("account.hcl"))
  region_vars  = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  env_vars     = read_terragrunt_config(find_in_parent_folders("env.hcl"))

  account_id  = local.account_vars.locals.account_id
  aws_region  = local.region_vars.locals.aws_region
  environment = local.env_vars.locals.environment
}
```

---

### sops_decrypt_file()

Decrypts **SOPS-encrypted files** (YAML, JSON, or binary).

```hcl
# terragrunt.hcl

locals {
  secrets = yamldecode(sops_decrypt_file("secrets.yaml.enc"))
}

inputs = {
  db_password = local.secrets.database_password
  api_key     = local.secrets.api_key
}
```

---

### Quick Reference

| Function                       | Purpose                                          |
| :----------------------------- | :----------------------------------------------- |
| **`get_env()`**                | Read environment variables with optional default |
| **`read_terragrunt_config()`** | Parse another Terragrunt file                    |
| **`sops_decrypt_file()`**      | Decrypt SOPS-encrypted secrets                   |

## Shell Execution

### run_cmd()

Runs a shell command and **captures the output**.

A common use case is fetching secrets from AWS Secrets Manager:

```hcl
# terragrunt.hcl

locals {
  db_password = run_cmd(
    "--terragrunt-quiet",
    "aws", "secretsmanager", "get-secret-value",
    "--secret-id", "prod/db/password",
    "--query", "SecretString",
    "--output", "text"
  )
}

inputs = {
  database_password = local.db_password
}
```

---

### The `--terragrunt-quiet` Flag

By default, `run_cmd` echoes the command's stdout to Terragrunt's logs **in addition** to returning it as the result of the expression.

The `--terragrunt-quiet` flag **redacts that stdout from the logs** while still returning the value to HCL — keeping secrets and other sensitive output out of log files.

> The returned value is the same with or without the flag; what changes is whether the output is also visible in Terragrunt's logs.

---

### Available Flags

| Flag                            | What it does                                                                    |
| :------------------------------ | :------------------------------------------------------------------------------ |
| **`--terragrunt-quiet`**        | Redacts the command's stdout from Terragrunt logs *(useful for sensitive data)* |
| **`--terragrunt-global-cache`** | Caches globally *(runs once across all configs)*                                |
| **`--terragrunt-no-cache`**     | Disables caching when you need fresh values                                     |

> See the [official documentation](/reference/hcl/functions/#special-parameters) for additional details on these parameters.

## Command Helpers

These functions return **lists of commands** that accept certain flags.

Pair them with **`extra_arguments`**:

```hcl
# terragrunt.hcl

terraform {
  extra_arguments "vars" {
    commands  = get_terraform_commands_that_need_vars()
    arguments = ["-var-file=${get_terragrunt_dir()}/extra.tfvars"]
  }

  extra_arguments "lock_timeout" {
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }

  extra_arguments "parallelism" {
    commands  = get_terraform_commands_that_need_parallelism()
    arguments = ["-parallelism=5"]
  }
}
```

---

### Available Helper Functions

| Function                                             | Returns Commands That...            |
| :--------------------------------------------------- | :---------------------------------- |
| **`get_terraform_commands_that_need_vars()`**        | Accept `-var` and `-var-file` flags |
| **`get_terraform_commands_that_need_locking()`**     | Use state locking                   |
| **`get_terraform_commands_that_need_parallelism()`** | Support the `-parallelism` flag     |

## Locals and mark_as_read()

### Locals

The `locals` block defines values you can **reuse** throughout your configuration.

Access them with `local.<name>`.

```hcl
# terragrunt.hcl

locals {
  account_vars = read_terragrunt_config(find_in_parent_folders("account.hcl"))
  region_vars  = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  env_vars     = read_terragrunt_config(find_in_parent_folders("env.hcl"))

  account_id  = local.account_vars.locals.account_id
  aws_region  = local.region_vars.locals.aws_region
  environment = local.env_vars.locals.environment

  common_tags = {
    Environment = local.environment
    Region      = local.aws_region
    ManagedBy   = "Terragrunt"
    GitCommit   = run_cmd("git", "rev-parse", "--short", "HEAD")
  }
}
```

> Locals get parsed **early**, so they're a good place for computed values you'll reference elsewhere.

---

### mark_as_read()

When you read files with `file()`, Terragrunt **doesn't automatically track** them as dependencies.

This matters when using `--filter 'reading=<file>'` to selectively run units that read specific files.

The `mark_as_read()` function **registers the file** as having been read by the unit. It returns the file path, so you wrap it around the path you pass to `file()`:

`mark_as_read()` effectively enables Terragrunt to add units to a run queue dependent on non-HCL files.

```hcl
# terragrunt.hcl

locals {
  # Without mark_as_read - Terragrunt doesn't know this unit reads the file
  policy = jsondecode(file("${get_repo_root()}/policies/s3-read-only.json"))

  # With mark_as_read - Terragrunt tracks this file as a dependency
  policy_path    = "${get_repo_root()}/policies/s3-read-only.json"
  tracked_policy = jsondecode(file(mark_as_read(local.policy_path)))
}
```

---

#### Key Points

| Requirement           | Why                                                                   |
| :-------------------- | :-------------------------------------------------------------------- |
| Use `mark_as_read()`  | Enables tracking non-HCL files                                        |
| Must be in `locals`   | For filter tracking to work                                           |
| Works with `--filter` | `--filter 'reading=<file>'` to include units that read specific files |

---

#### When to Use `mark_as_read()`

| Use Case              | Example                                                                              |
| :-------------------- | :----------------------------------------------------------------------------------- |
| Reading non-HCL files | JSON, YAML, or other files loaded with `file()` that affect your infrastructure      |
| Filter detection      | When you need `--filter 'reading=<file>'` to scope `run` commands by file dependency |

> Terragrunt Functions like `read_terragrunt_config()`, `read_tfvars_file()`, and `sops_decrypt_file()` **automatically track** their files — you don't need `mark_as_read()` for those.
