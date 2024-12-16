---
layout: collection-browser-doc
title: Before, After, and Error Hooks
category: features
categories_url: features
excerpt: Learn how to execute custom code before or after running OpenTofu/Terraform, or when errors occur.
tags: ["hooks"]
order: 240
nav_title: Documentation
nav_title_link: /docs/
---

_Before Hooks_, _After Hooks_ and _Error Hooks_ are a feature of terragrunt that make it possible to define custom actions that will be called before or after running an `tofu`/`terraform` command.

They allow you to _orchestrate_ certain operations around IaC updates so that you have a consistent way to run custom code before or after running OpenTofu/Terraform.

Here’s an example:

``` hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running OpenTofu"]
  }

  after_hook "after_hook" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Finished running OpenTofu"]
    run_on_error = true
  }

  error_hook "import_resource" {
    commands  = ["apply"]
    execute   = ["echo", "Error Hook executed"]
    on_errors = [
      ".*",
    ]
  }
}
```

In this example configuration, whenever Terragrunt runs `tofu apply` or `tofu plan` (or the `terraform` equivalent), three things will happen:

- Before Terragrunt runs `tofu`/`terraform`, it will output `Running OpenTofu` to the console.
- After Terragrunt runs `tofu`/`terraform`, it will output `Finished running OpenTofu`, regardless of whether or not the
  command failed.
- If an error occurs during the `tofu apply` command, Terragrunt will output `Error Hook executed`.

You can learn more about all the various configuration options supported in [the reference docs for the terraform
block](/docs/reference/config-blocks-and-attributes/#terraform).

## Hook Context

All hooks add extra environment variables when executing the hook's run command:

- `TG_CTX_TF_PATH`
- `TG_CTX_COMMAND`
- `TG_CTX_HOOK_NAME`

For example:

```hcl
terraform {
  before_hook "test_hook" {
    commands     = ["apply"]
    execute      = ["hook.sh"]
  }
}
```

Where `hook.sh` is:

``` bash
#!/bin/sh

echo "TF_PATH=${TG_CTX_TF_PATH} COMMAND=${TG_CTX_COMMAND} HOOK_NAME=${TG_CTX_HOOK_NAME}"
```

Will result in the following output when running `tofu apply`/`terraform apply`:

``` bash
TF_PATH=tofu COMMAND=apply HOOK_NAME=test_hook
```

Note that hooks are executed within the working directory where OpenTofu/Terraform would be run.

If using the `source` attribute for the `terraform` block, this will result in the hook running in
the hidden `.terragrunt-cache` directory.

This also means that you can use `tofu`/`terraform` commands within hooks to access any outputs needed
for hook logic.

For example:

```bash
#!/usr/bin/env bash

# Get the bucket_name output from OpenTofu/Terraform state
BUCKET_NAME="$("$TG_CTX_TF_PATH" output -raw bucket_name)"

# Use the AWS CLI to list the contents of the bucket
aws s3 ls "s3://$BUCKET_NAME"
```

Note that the `TG_CTX_TF_PATH` environment variable is used here to ensure compatibility, regardless of the
value of [terragrunt-tfpath](/docs/reference/cli-options/#terragrunt-tfpath). This can be a useful practice
if you are migrating between OpenTofu or Terraform.

You will also have access to all the `inputs` set in the `terragrunt.hcl` file as environment variables prefixed
by `TF_VAR_`, as that's how the variables are set for use in OpenTofu/Terraform.

For example, if you have the following `inputs` block in your `terragrunt.hcl` file:

```hcl
inputs = {
  bucket_name = "my-bucket"
}
```

You can access the `bucket_name` input in your hook as follows:

```bash
#!/usr/bin/env bash

# Get the bucket_name input from the terragrunt.hcl file
BUCKET_NAME="$TF_VAR_bucket_name"

# Use the AWS CLI to list the contents of the bucket
aws s3 ls "s3://$BUCKET_NAME"
```

## Orchestrating execution outside IaC

Hooks can be used to handle operations that need to happen, but are not directly related to the OpenTofu/Terraform.

For example, you may be using Terragrunt to manage an [AWS ECS service](https://aws.amazon.com/ecs/).

You can use a `before_hook` to build and push a new image to the [Elastic Container Registry (ECR)](https://aws.amazon.com/ecr/) before running `tofu apply`.

```hcl
terraform {
  before_hook "build_and_push_image" {
    commands     = ["plan", "apply"]
    execute      = ["./build_and_push_image.sh"]
  }
}
```

Where `build_and_push_image.sh` is something like:

```bash
#!/usr/bin/env bash

set -eou pipefail

ACCOUNT_ID="123456789012"
REGION="us-east-1"
REPOSITORY="my-repository"
TAG="latest"

IMAGE_TAG="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com/${REPOSITORY}:${TAG}"

# Build the Docker image
docker build -t "$IMAGE_TAG" .

# Push the Docker image to ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-1.amazonaws.com
docker push "$IMAGE_TAG"
```

The hard-coding of values in the script could be replaced with context as shown in the previous section.

Similarly, you may want to smoke-test newly deployed infrastructure after running `tofu apply`.

```hcl
terraform {
  after_hook "smoke_test" {
    commands     = ["apply"]
    execute      = ["./smoke_test.sh"]
    run_on_error = true
  }
}
```

Where `smoke_test.sh` is something like:

```bash
#!/usr/bin/env bash

set -eou pipefail

# Get the URL for the service from OpenTofu/Terraform state
SERVICE_URL="$("$TG_CTX_TF_PATH" output -raw service_url)"

# Use curl to check the service is up
curl -sSf "$SERVICE_URL"
```

You might even decide to integrate with a product like [Terratest](https://github.com/gruntwork-io/terratest) for more complex testing.

## Hook Ordering

You can have multiple before and after hooks. Each hook will execute in the order they are defined.

For example:

```hcl
terraform {
  before_hook "before_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Will run OpenTofu"]
  }

  before_hook "before_hook_2" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Running OpenTofu"]
  }
}
```

This configuration will cause Terragrunt to output `Will run OpenTofu` and then `Running OpenTofu` before the call
to OpenTofu/Terraform.

## Tflint hook

_Before Hooks_ or _After Hooks_ natively support _tflint_, a linter for OpenTofu/Terraform code. It will validate the
OpenTofu/Terraform code used by Terragrunt, and it's inputs.

Here's an example:

```hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["tflint"]
  }
}
```

The `.tflint.hcl` should exist in the same folder as `terragrunt.hcl` or one of it's parents. If Terragrunt can't find
a `.tflint.hcl` file, it won't execute tflint and return an error. All configurations should be in a `config` block in this
file, as per [Tflint's docs](https://github.com/terraform-linters/tflint/blob/master/docs/user-guide/config.md).

```hcl
plugin "aws" {
    enabled = true
    version = "0.21.0"
    source  = "github.com/terraform-linters/tflint-ruleset-aws"
}

config {
  module = true
}
```

### Configuration

By default, `tflint` is executed with the internal `tflint` built into Terragrunt, which will evaluate parameters passed in.

Any desired extra configuration should be added in the `.tflint.hcl` file.
It will work with a `.tflint.hcl` file in the current folder or any parent folder.
To utilize an alternative configuration file, use the `--config` flag with the path to the configuration file.

If there is a need to run `tflint` from the operating system directly, use the extra parameter `--terragrunt-external-tflint`.
This will result in usage of the `tflint` binary found in the `PATH` environment variable.

For example:

```hcl
terraform {
    before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint" , "--terragrunt-external-tflint", "--minimum-failure-severity=error", "--config", "custom.tflint.hcl"]
  }
}
```

### Authentication for tflint rulesets

<!-- markdownlint-disable MD036 -->
_Public rulesets_

`tflint` works without any authentication for public rulesets (hosted on public repositories).

_Private rulesets_

If you want to run a the `tflint` hook with custom rulesets defined in a private repository, you will need to export a valid `GITHUB_TOKEN` token.

### Troubleshooting

__`flag provided but not defined: -act-as-bundled-plugin` error__

If you have an `.tflint.hcl` file that is empty, or uses the `terraform` ruleset without version or source constraint, it can return the following error:

```log
Failed to initialize plugins; Unrecognized remote plugin message: Incorrect Usage. flag provided but not defined: -act-as-bundled-plugin
```

To fix this, make sure that the configuration for the `terraform` ruleset, in the `.tflint.hcl` file contains a version constraint:

```hcl
plugin "terraform" {
    enabled = true
    version = "0.2.1"
    source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}
```
