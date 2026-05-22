---
title: Advanced Patterns
description: Dynamic credentials, efficient run queues, and error handling for transient failures
slug: guides/terragrunt-101/advanced-patterns
sidebar:
  order: 8
next: false
---

## Learning Objectives

Upon completing this module, you will be able to:

| Objective                         | What You'll Learn                      |
| :-------------------------------- | :------------------------------------- |
| **Configure dynamic credentials** | Using `--auth-provider-cmd`            |
| **Execute efficiently**           | Run queue strategies with filtering    |
| **Handle errors**                 | Automatic retry for transient failures |

## Dynamic Credentials with `--auth-provider-cmd`

The `--auth-provider-cmd` flag enables Terragrunt to obtain AWS credentials **dynamically** by executing a command that returns credentials in JSON format.

This approach works with **any credential provider**:

- AWS SSO
- OIDC token exchanges
- Custom scripts

---

### Configuration

Configure `--auth-provider-cmd` via CLI flag or environment variable:

```bash
# CLI flag
terragrunt apply --auth-provider-cmd /path/to/get-aws-creds.sh

# Or set as environment variable
export TG_AUTH_PROVIDER_CMD="/path/to/get-aws-creds.sh"
terragrunt apply
```

Terragrunt runs this command whenever it needs authentication, including during HCL parsing (for functions like `get_aws_account_id`) and before executing OpenTofu/Terraform.

---

### Expected Output Format

The command must output JSON matching this schema (all fields optional):

```json
{
  "awsCredentials": {
    "ACCESS_KEY_ID": "ASIA...",
    "SECRET_ACCESS_KEY": "...",
    "SESSION_TOKEN": "..."
  },
  "awsRole": {
    "roleARN": "arn:aws:iam::123456789012:role/MyRole",
    "sessionName": "terragrunt-session",
    "duration": 3600,
    "webIdentityToken": ""
  },
  "envs": {
    "CUSTOM_VAR": "value"
  }
}
```

| Field            | Description                                 |
| :--------------- | :------------------------------------------ |
| `awsCredentials` | Static AWS credentials to use directly      |
| `awsRole`        | Role to assume (Terragrunt handles refresh) |
| `envs`           | Additional environment variables to set     |

The `envs` field is useful for authenticating to **non-AWS providers**. For example, you can return credentials for Azure, GCP, or any other service that uses environment variables:

```json
{
  "envs": {
    "ARM_CLIENT_ID": "...",
    "ARM_CLIENT_SECRET": "...",
    "ARM_TENANT_ID": "...",
    "GOOGLE_CREDENTIALS": "..."
  }
}
```

---

### AWS SSO Example

A common use case is obtaining credentials from **AWS SSO (Identity Center)**:

```bash
#!/bin/bash
# get-aws-creds.sh - Wrapper for AWS SSO credentials

PROFILE="${AWS_PROFILE:-default}"

# Ensure SSO session is active
aws sso login --profile "$PROFILE" 2>/dev/null || true

# Export credentials in the expected JSON format
aws configure export-credentials | \
  jq '{
    awsCredentials: {
      ACCESS_KEY_ID: .AccessKeyId,
      SECRET_ACCESS_KEY: .SecretAccessKey,
      SESSION_TOKEN: .SessionToken
    }
  }'
```

---

### Benefits

| Benefit                   | Description                                   |
| :------------------------ | :-------------------------------------------- |
| **Dynamic credentials**   | Fetched at runtime by a command you control   |
| **No static credentials** | Avoids secrets in configuration files         |
| **Flexible**              | Works with any credential provider            |
| **CI/CD friendly**        | Different roles for different pipeline stages |
| **Multi-account support** | Assume different roles per account            |

:::caution
**`--auth-provider-cmd` provides _dynamic_ credentials, not automatically _secure_ ones.** Terragrunt runs the command you give it and uses whatever it returns. It's your responsibility to write a script that fetches credentials securely. For example, source short-lived credentials from AWS SSO or OIDC, or return an `awsRole` so Terragrunt assumes and refreshes the role for you.
:::

## Run Strategies and Filtering

### Parallel Execution

Control how many units run simultaneously with **`--parallelism`**:

```bash
# Run up to 10 units in parallel
terragrunt run apply --all --parallelism 10
```

| Value       | Effect                      |
| :---------- | :-------------------------- |
| **Lower**   | Reduces AWS API throttling  |
| **Higher**  | Speeds up large deployments |
| **Default** | 10                          |

---

### Filtering Units

Target specific units without running the entire stack using the **`--filter`** flag:

```bash
# Include only networking units
terragrunt run --filter '{./*/networking/*}' -- plan

# Exclude deprecated or test units
terragrunt run --filter '!{./*/deprecated/*}' -- plan

# Combine filters with intersection
terragrunt run --filter '{./prod/*} | !{./*/test/*}' -- plan

# Filter by files read (useful for shared configs)
terragrunt run --filter 'reading=root.hcl' -- plan
```

> The `--filter` flag **implies `--all`**, so you don't need to specify both.

---

### Filter Syntax Examples

The filter flag supports a **flexible query language**:

| Filter Type               | Example                           | Description               |
| :------------------------ | :-------------------------------- | :------------------------ |
| **Path-based**            | `'{./prod/**}'`                   | Glob patterns on paths    |
| **Name-based**            | `'app*'`                          | Match unit names          |
| **Type-based**            | `'type=unit'`                     | Filter by type            |
| **Negation**              | `'!{./test/**}'`                  | Exclude matches           |
| **Intersection** (AND)    | `'{./prod/**} \| type=unit'`      | Both must match           |
| **Multiple filters** (OR) | `--filter 'app1' --filter 'app2'` | Either can match          |
| **Dependencies**          | `'service...'`                    | Unit and its dependencies |
| **Dependents**            | `'...service'`                    | Unit and its dependents   |

---

#### Code Examples

```bash
# Path-based filtering with globs
terragrunt run --filter '{./prod/**}' -- plan

# Negation (exclude)
terragrunt run --filter '!{./test/**}' -- plan

# Intersection (AND logic)
terragrunt run --filter '{./prod/**} | type=unit' -- plan

# Multiple filters (OR logic)
terragrunt run --filter 'app1' --filter 'app2' -- plan

# Graph-based filtering (unit and its dependencies)
terragrunt run --filter 'service...' -- plan

# Graph-based filtering (unit and its dependents)
terragrunt run --filter '...service' -- plan
```

## Error Handling

### Automatic Retry for Transient Errors

AWS API calls occasionally fail due to:

- Rate limiting
- Eventual consistency
- Network issues

Configure **automatic retries**:

```hcl
# root.hcl
errors {
  retry "transient_aws_errors" {
    retryable_errors   = get_default_retryable_errors()
    max_attempts       = 5
    sleep_interval_sec = 10
  }
}
```

---

### Adding Custom Error Patterns

```hcl
errors {
  retry "rate_limits" {
    retryable_errors = concat(
      get_default_retryable_errors(),
      [".*Throttling.*", ".*Rate exceeded.*"]
    )
    max_attempts       = 5
    sleep_interval_sec = 30
  }
}
```

---

### Retry Configuration Options

| Attribute                | Description                      |
| :----------------------- | :------------------------------- |
| **`retryable_errors`**   | List of regex patterns to match  |
| **`max_attempts`**       | Maximum number of retry attempts |
| **`sleep_interval_sec`** | Seconds to wait between retries  |

---

### Ignoring Expected Errors

Some errors are expected and **safe to ignore**, like deleting resources that no longer exist:

```hcl
errors {
  ignore "already_deleted" {
    ignorable_errors = [
      ".*ResourceNotFoundException.*",
      ".*NoSuchEntity.*"
    ]
    message = "Resource already deleted, continuing"
  }
}
```

| Use Case             | Pattern Example                   |
| :------------------- | :-------------------------------- |
| **Resource deleted** | `".*ResourceNotFoundException.*"` |
| **IAM entity gone**  | `".*NoSuchEntity.*"`              |
| **Not found**        | `".*NotFound.*"`                  |
