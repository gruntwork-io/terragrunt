---
layout: collection-browser-doc
title: Keep your remote state configuration DRY
category: features
categories_url: features
excerpt: Learn how to create and manage remote state configuration.
tags: ["DRY", "remote", "Use cases", "CLI"]
order: 205
nav_title: Documentation
nav_title_link: /docs/
---

## Keep your remote state configuration DRY

  - [Motivation](#motivation)

  - [Filling in remote state settings with Terragrunt](#filling-in-remote-state-settings-with-terragrunt)

  - [Create remote state and locking resources automatically](#create-remote-state-and-locking-resources-automatically)


### Motivation

Terraform supports [remote state storage](https://www.terraform.io/docs/state/remote.html) via a variety of [backends](https://www.terraform.io/docs/backends) that you normally configure in your `.tf` files as follows:

``` hcl
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "frontend-app/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

Unfortunately, the `backend` configuration does not support expressions, variables, or functions. This makes it hard to keep your code [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) if you have multiple Terraform modules. For example, consider the following folder structure, which uses different Terraform modules to deploy a backend app, frontend app, MySQL database, and a VPC:

    ├── backend-app
    │   └── main.tf
    ├── frontend-app
    │   └── main.tf
    ├── mysql
    │   └── main.tf
    └── vpc
        └── main.tf

To use remote state with each of these modules, you would have to copy/paste the exact same `backend` configuration into each of the `main.tf` files. The only thing that would differ between the configurations would be the `key` parameter: e.g., the `key` for `mysql/main.tf` might be `mysql/terraform.tfstate` and the `key` for `frontend-app/main.tf` might be `frontend-app/terraform.tfstate`.

To keep your remote state configuration DRY, you can use Terragrunt.

### Filling in remote state settings with Terragrunt

To fill in the settings via Terragrunt, create a `terragrunt.hcl` file in the root folder, plus one `terragrunt.hcl` file in each of the Terraform modules:

    ├── terragrunt.hcl
    ├── backend-app
    │   ├── main.tf
    │   └── terragrunt.hcl
    ├── frontend-app
    │   ├── main.tf
    │   └── terragrunt.hcl
    ├── mysql
    │   ├── main.tf
    │   └── terragrunt.hcl
    └── vpc
        ├── main.tf
        └── terragrunt.hcl

In your **root** `terragrunt.hcl` file, you can define your entire remote state configuration just once in a `generate` block, to generate a `backend.tf` file that includes the backend configuration:

```hcl
generate "backend" {
  path      = "backend.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
terraform {
  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
EOF
}
```

This instructs Terragrunt to create the file `backend.tf` in the working directory (where Terragrunt calls `terraform`)
before it calls any of the Terraform commands, including `init`. This allows you to inject this backend configuration
in all the modules that includes the root file and have `terragrunt` properly initialize the backend configuration with
interpolated values.

To inherit this configuration, in each of the **child** `terragrunt.hcl` files, such as `mysql/terragrunt.hcl`, you can
tell Terragrunt to automatically include all the settings from the root `terragrunt.hcl` file as follows:

``` hcl
include "root" {
  path = find_in_parent_folders()
}
```

The `include` block tells Terragrunt to use the exact same Terragrunt configuration from the `terragrunt.hcl` file specified via the `path` parameter. It behaves exactly as if you had copy/pasted the Terraform configuration from the included file `generate` configuration into `mysql/terragrunt.hcl`, but this approach is much easier to maintain\!

The next time you run `terragrunt`, it will automatically configure all the settings for the backend, if they aren’t configured already, by calling [terraform init](https://www.terraform.io/docs/commands/init.html).

The `terragrunt.hcl` files above use two Terragrunt built-in functions:

  - `find_in_parent_folders()`: This function returns the absolute path to the first `terragrunt.hcl` file it finds in the parent folders above the current `terragrunt.hcl` file. In the example above, the call to `find_in_parent_folders()` in `mysql/terragrunt.hcl` will return `/your-root-folder/terragrunt.hcl`. This way, you don’t have to hard code the `path` parameter in every module.

  - `path_relative_to_include()`: This function returns the relative path between the current `terragrunt.hcl` file and the path specified in its `include` block. We typically use this in a root `terragrunt.hcl` file so that each Terraform child module stores its Terraform state at a different `key`. For example, the `mysql` module will have its `key` parameter resolve to `mysql/terraform.tfstate` and the `frontend-app` module will have its `key` parameter resolve to `frontend-app/terraform.tfstate`.

See [the Built-in Functions docs]({{site.baseurl}}/docs/reference/built-in-functions/#built-in-functions) for more info.


### Create remote state and locking resources automatically

The `generate` block is useful for allowing you to setup the remote state backend configuration in a DRY manner, but
this introduces a bootstrapping problem: how do you create and manage the underlying storage resources for the remote
state? For example, when using the [s3 backend](https://www.terraform.io/language/settings/backends/s3), Terraform
expects the S3 bucket to already exist for it to upload the state objects.

Ideally you can manage the S3 bucket using Terraform, but what about the state object for the module managing the S3
bucket? How do you create the S3 bucket, before you run `terraform`, if you need to run `terraform` to create the
bucket?

To handle this, Terragrunt supports a different block for managing the backend configuration: the [remote_state
block](https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes/#remote_state).

> **NOTE**
>
> `remote_state` is an alternative way of managing the Terraform backend compared to `generate`. You can not use both
> methods at the same time to manage the remote state configuration. When implementing `remote_state`, be sure to remove
> the corresponding `generate` block for managing the backend.

The following backends are currently supported by `remote_state`:

- [s3 backend](https://www.terraform.io/language/settings/backends/s3)
- [gcs backend](https://www.terraform.io/language/settings/backends/gcs)

For all other backends, the `remote_state` block operates in the same manner as `generate`. However, we may add
support for additional backends to `remote_state` blocks, which may disrupt your environment. If you do not want support
for automated management of remote state resources, we recommend sticking to `generate` blocks to configure the backend.

When you run `terragrunt` with a `remote_state` configuration, it will automatically create the following resources if they don’t already exist:

  - **S3 bucket**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for remote state storage and the `bucket` you specify in `remote_state.config` doesn’t already exist, Terragrunt will create it automatically, with [versioning](https://docs.aws.amazon.com/AmazonS3/latest/dev/Versioning.html), [server-side encryption](https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingServerSideEncryption.html), and [access logging](https://docs.aws.amazon.com/AmazonS3/latest/dev/ServerLogs.html) enabled.

    In addition, you can let terragrunt tag the bucket with custom tags that you specify in `remote_state.config.s3_bucket_tags`.

  - **DynamoDB table**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for remote state storage and you specify a `dynamodb_table` (a [DynamoDB table used for locking](https://www.terraform.io/docs/backends/types/s3.html#dynamodb_table)) in `remote_state.config`, if that table doesn’t already exist, Terragrunt will create it automatically, with [server-side encryption](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/EncryptionAtRest.html) enabled, including a primary key called `LockID`.

    You may configure custom endpoint for the AWS DynamoDB API using `remote_state.config.dynamodb_endpoint`.

    In addition, you can let terragrunt tag the DynamoDB table with custom tags that you specify in `remote_state.config.dynamodb_table_tags`.

  - **GCS bucket**: If you are using the [GCS backend](https://www.terraform.io/docs/backends/types/gcs.html) for remote state storage and the `bucket` you specify in `remote_state.config` doesn’t already exist, Terragrunt will create it automatically, with [versioning](https://cloud.google.com/storage/docs/object-versioning) enabled. For this to work correctly you must also specify `project` and `location` keys in `remote_state.config`, so Terragrunt knows where to create the bucket. You will also need to supply valid credentials using either `remote_state.config.credentials` or by setting the `GOOGLE_APPLICATION_CREDENTIALS` environment variable. If you want to skip creating the bucket entirely, simply set `skip_bucket_creation` to `true` and Terragrunt will assume the bucket has already been created. If you don’t specify `bucket` in `remote_state` then terragrunt will assume that you will pass `bucket` through `-backend-config` in `extra_arguments`.

    We also strongly recommend you enable [Cloud Audit Logs](https://cloud.google.com/storage/docs/access-logs) to audit and track API operations performed against the state bucket.

    In addition, you can let Terragrunt label the bucket with custom labels that you specify in `remote_state.config.gcs_bucket_labels`.

**Note**: If you specify a `profile` key in `remote_state.config`, Terragrunt will automatically use this AWS profile when creating the S3 bucket or DynamoDB table.

**Note**: You can disable automatic remote state initialization by setting `remote_state.disable_init`, this will skip the automatic creation of remote state resources and will execute `terraform init` passing the `backend=false` option. This can be handy when running commands such as `validate-all` as part of a CI process where you do not want to initialize remote state.

The following example demonstrates using an environment variable to configure this option:

``` hcl
remote_state {
  # ...

  disable_init = tobool(get_env("TERRAGRUNT_DISABLE_INIT", "false"))
}
```

Here is an example of using the `remote_state` block to configure the S3 backend:

```hcl
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket         = "my-terraform-state"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

Like the approach with `generate` blocks, this will generate a `backend.tf` file that contains the remote state
configuration. However, in addition to that, `terragrunt` will also now manage the S3 bucket and DynamoDB table for you.
This means that if the S3 bucket `my-terraform-state` and DynamoDB table `my-lock-table` does not exist in your account,
Terragrunt will automatically create these resources before calling `terraform` and configure them based on the
specified configuration parameters.

Additionally, for **the S3 backend only**, Terragrunt will automatically update the S3 resource to match the
configuration specified in the `remote_state` bucket. For example, if you require versioning in the `remote_state`
block, but the underlying state bucket doesn't have versioning enabled, Terragrunt will automatically turn on versioning
on the bucket to match the configuration.

If you do not want `terragrunt` to automatically apply changes, you can configure the following:

```hcl
remote_state {
  # ... other args omitted for brevity ...
  config = {
    # ... other config omitted for brevity ...
    disable_bucket_update = true
  }
}
```

Check out the [terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example) and [terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) repos for fully-working sample code that demonstrates how to use Terragrunt to manage remote state.

### S3-specific remote state settings

For the `s3` backend, the following config options can be used for S3-compatible object stores, as necessary:

**Note**: The `skip_bucket_accesslogging` is now DEPRECATED. It is replaced by `accesslogging_bucket_name`. Please read below for more details on when to use the new config option.

``` hcl
remote_state {
  # ...

  config = {
    skip_bucket_versioning         = true # use only if the object store does not support versioning
    skip_bucket_ssencryption       = true # use only if non-encrypted Terraform State is required and/or the object store does not support server-side encryption
    skip_bucket_root_access        = true # use only if the AWS account root user should not have access to the remote state bucket for some reason
    skip_bucket_enforced_tls       = true # use only if you need to access the S3 bucket without TLS being enforced
    enable_lock_table_ssencryption = true # use only if non-encrypted DynamoDB Lock Table for the Terraform State is required and/or the NoSQL database service does not support server-side encryption
    accesslogging_bucket_name      = <string> # use only if you need server access logging to be enabled for your terraform state S3 bucket. Provide a <string> value representing the name of the target bucket to be used for logs output.
    accesslogging_target_prefix    = <string> # use only if you want to set a specific prefix for your terraform state S3 bucket access logs when Server Access Logging is enabled. Provide a <string> value representing the TargetPrefix to be used for the logs output objects. If set to empty <string>, then TargetPrefix will be set to empty <string>. If attribute is not provided at all, then TargetPrefix will be set to default value `TFStateLogs/`.

    shared_credentials_file     = "/path/to/credentials/file"
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    force_path_style            = true
  }
}
```

If you experience an error for any of these configurations, confirm you are using Terraform v0.12.2 or greater.

Further, the config options `s3_bucket_tags`, `dynamodb_table_tags`, `skip_bucket_versioning`, `skip_bucket_ssencryption`, `skip_bucket_root_access`, `skip_bucket_enforced_tls`, `skip_bucket_public_access_blocking`, `accesslogging_bucket_name`, `accesslogging_target_prefix`, and `enable_lock_table_ssencryption` are only valid for backend `s3`. They are used by terragrunt and are **not** passed on to terraform. See section [Create remote state and locking resources automatically](#create-remote-state-and-locking-resources-automatically).

### GCS-specific remote state settings

For the `gcs` backend, the following config options can be used for GCS-compatible object stores, as necessary:

``` hcl
remote_state {
 # ...

 skip_bucket_versioning = true # use only if the object store does not support versioning

 enable_bucket_policy_only = false # use only if uniform bucket-level access is needed (https://cloud.google.com/storage/docs/uniform-bucket-level-access)

 encryption_key = "GOOGLE_ENCRYPTION_KEY"
}
```

If you experience an error for any of these configurations, confirm you are using Terraform v0.12.0 or greater.

Further, the config options `gcs_bucket_labels`, `skip_bucket_versioning` and `enable_bucket_policy_only` are only valid for the backend `gcs`. They are used by terragrunt and are **not** passed on to terraform. See section [Create remote state and locking resources automatically](#create-remote-state-and-locking-resources-automatically).
