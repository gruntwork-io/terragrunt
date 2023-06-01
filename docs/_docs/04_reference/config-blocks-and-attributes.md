---
layout: collection-browser-doc
title: Configuration Blocks and Attributes
category: reference
categories_url: reference
excerpt: >-
  Learn about all the blocks and attributes supported in the terragrunt configuration file.
tags: ["config"]
order: 403
nav_title: Documentation
nav_title_link: /docs/
---

The Terragrunt configuration file uses the same HCL syntax as Terraform itself in `terragrunt.hcl`.
Terragrunt also supports [JSON-serialized HCL](https://github.com/hashicorp/hcl/blob/hcl2/json/spec.md) in a `terragrunt.hcl.json` file:
where `terragrunt.hcl` is mentioned you can always use `terragrunt.hcl.json` instead.

The following is a reference of all the supported blocks and attributes in the configuration file:

  - [Blocks](#blocks)
  - [Attributes](#attributes)

## Blocks

- [terraform](#terraform)
- [remote_state](#remote_state)
- [include](#include)
- [locals](#locals)
- [dependency](#dependency)
- [dependencies](#dependencies)
- [generate](#generate)

### terraform

The `terraform` block is used to configure how Terragrunt will interact with Terraform. This includes specifying where
to find the Terraform configuration files, any extra arguments to pass to the `terraform` CLI, and any hooks to run
before or after calling Terraform.

The `terraform` block supports the following arguments:

- `source` (attribute): Specifies where to find Terraform configuration files. This parameter supports the exact same syntax as the
  [module source](https://www.terraform.io/docs/modules/sources.html) parameter for Terraform `module` blocks **except
  for the Terraform registry** (see below note), including local file paths, Git URLs, and Git URLS with `ref`
  parameters. Terragrunt will download all the code in the repo (i.e. the part before the double-slash `//`) so that
  relative paths work correctly between modules in that repo.
    - The `source` parameter can be configured to pull Terraform modules from any Terraform module registry using
      the `tfr` protocol. The `tfr` protocol expects URLs to be provided in the format
      `tfr://REGISTRY_HOST/MODULE_SOURCE?version=VERSION`. For example, to pull the `terraform-aws-modules/vpc/aws`
      module from the public Terraform registry, you can use the following as the source parameter:
      `tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=3.3.0`.
    - If you wish to access a private module registry (e.g., [Terraform Cloud/Enterprise](https://www.terraform.io/docs/cloud/registry/index.html)),
      you can provide the authentication to Terragrunt as an environment variable with the key `TG_TF_REGISTRY_TOKEN`.
      This token can be any registry API token.
    - The `tfr` protocol supports a shorthand notation where the `REGISTRY_HOST` can be omitted to default to the public
      registry (`registry.terraform.io`) if you use `tfr:///` (note the three `/`). For example, the following will
      fetch the `terraform-aws-modules/vpc/aws` module from the public registry:
      `tfr:///terraform-aws-modules/vpc/aws?version=3.3.0`.
    - You can also use submodules from the registry using `//`. For example, to use the `iam-policy` submodule from the
      registry module
      [terraform-aws-modules/iam](https://registry.terraform.io/modules/terraform-aws-modules/iam/aws/latest), you can
      use the following: `tfr:///terraform-aws-modules/iam/aws//modules/iam-policy?version=4.3.0`.
    - Refer to [A note about using modules from the
      registry]({{site.baseurl}}/docs/getting-started/quick-start#a-note-about-using-modules-from-the-registry) for more
      information about using modules from the Terraform Registry with Terragrunt.

- `include_in_copy` (attribute): A list of glob patterns (e.g., `["*.txt"]`) that should always be copied into the
  Terraform working directory. When you use the `source` param in your Terragrunt config and run `terragrunt <command>`,
  Terragrunt will download the code specified at source into a scratch folder (`.terragrunt-cache`, by default), copy
  the code in your current working directory into the same scratch folder, and then run `terraform <command>` in that
  scratch folder. By default, Terragrunt excludes hidden files and folders during the copy step. This feature allows you
  to specify glob patterns of files that should always be copied from the Terragrunt working directory. Additional
  notes:
    - The path should be specified relative to the source directory.
    - This list is also used when using a local file source (e.g., `source = "../modules/vpc"`). For example, if your
      terraform module source contains a hidden file that you want to copy over (e.g., a `.python-version` file), you
      can specify that in this list to ensure it gets copied over to the scratch copy
      (e.g., `include_in_copy = [".python-version"]`).

- `extra_arguments` (block): Nested blocks used to specify extra CLI arguments to pass to the `terraform` CLI. Learn more
  about its usage in the [Keep your CLI flags DRY]({{site.baseurl}}/docs/features/keep-your-cli-flags-dry/) use case overview. Supports
  the following arguments:
    - `arguments` (required) : A list of CLI arguments to pass to `terraform`.
    - `commands` (required) : A list of `terraform` sub commands that the arguments will be passed to.
    - `env_vars` (optional) : A map of key value pairs to set as environment variables when calling `terraform`.
    - `required_var_files` (optional): A list of file paths to terraform vars files (`.tfvars`) that will be passed in to
      `terraform` as `-var-file=<your file>`.
    - `optional_var_files` (optional): A list of file paths to terraform vars files (`.tfvars`) that will be passed in to
      `terraform` like `required_var_files`, only any files that do not exist are ignored.

- `before_hook` (block): Nested blocks used to specify command hooks that should be run before `terraform` is called.
  Hooks run from the directory with the terraform module, except for hooks related to `terragrunt-read-config` and
  `init-from-module`. These hooks run in the terragrunt configuration directory (the directory where `terragrunt.hcl`
  lives).
  Supports the following arguments:
    - `commands` (required) : A list of `terraform` sub commands for which the hook should run before.
    - `execute` (required) : A list of command and arguments that should be run as the hook. For example, if `execute` is set as
      `["echo", "Foo"]`, the command `echo Foo` will be run.
    - `working_dir` (optional) : The path to set as the working directory of the hook. Terragrunt will switch directory
      to this path prior to running the hook command. Defaults to the terragrunt configuration directory for
      `terragrunt-read-config` and `init-from-module` hooks, and the terraform module directory for other command hooks.
    - `run_on_error` (optional) : If set to true, this hook will run even if a previous hook hit an error, or in the
      case of "after" hooks, if the Terraform command hit an error. Default is false.
    - `suppress_stdout` (optional) : If set to true, the stdout output of the executed commands will be suppressed. This can be useful when there are scripts relying on terraform's output and any other output would break their parsing.

- `after_hook` (block): Nested blocks used to specify command hooks that should be run after `terraform` is called.
  Hooks run from the terragrunt configuration directory (the directory where `terragrunt.hcl` lives). Supports the same
  arguments as `before_hook`.
- `error_hook` (block): Nested blocks used to specify command hooks that run when an error is thrown. The
error must match one of the expressions listed in the `on_errors` attribute. Error hooks are executed after the before/after hooks.

In addition to supporting before and after hooks for all terraform commands, the following specialized hooks are also
supported:

- `terragrunt-read-config` (after hook only): `terragrunt-read-config` is a special hook command that you can use with
  the `after_hook` subblock to run an action immediately after terragrunt finishes loading the config. This hook will
  run on every invocation of terragrunt. Note that you can only use this hook with `after_hooks`. Any `before_hooks`
  with the command `terragrunt-read-config` will be ignored. The working directory for hooks associated with this
  command will be the terragrunt config directory.

- `init-from-module` and `init`: Terragrunt has two stages of initialization: one is to download [remote
  configurations](https://terragrunt.gruntwork.io/use-cases/keep-your-terraform-code-dry/) using `go-getter`; the other
  is [Auto-Init](https://terragrunt.gruntwork.io/docs/features/auto-init/), which configures the backend and downloads
  provider plugins and modules. If you wish to run a hook when Terragrunt is using `go-getter` to download remote
  configurations, use `init-from-module` for the command. If you wish to execute a hook when Terragrunt is using
  terraform `init` for Auto-Init, use `init` for the command. For example, an `after_hook` for the command
  `init-from-module` will run after terragrunt clones the module, while an `after_hook` for the command `init` will run
  after terragrunt runs `terraform init` on the cloned module.
    - Hooks for both `init-from-module` and `init` only run if the requisite stage needs to run. That is, if terragrunt
      detects that the module is already cloned in the terragrunt cache, this stage will be skipped and thus the hooks
      will not run. Similarly, if terragrunt detects that it does not need to run `init` in the auto init feature, the
      `init` stage is skipped along with the related hooks.
    - The working directory for hooks associated with `init-from-module` will run in the terragrunt config directory,
      while the working directory for hooks associated with `init` will be the terraform module.


Complete Example:

```hcl
terraform {
  # Pull the terraform configuration at the github repo "acme/infrastructure-modules", under the subdirectory
  # "networking/vpc", using the git tag "v0.0.1".
  source = "git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1"

  # For any terraform commands that use locking, make sure to configure a lock timeout of 20 minutes.
  extra_arguments "retry_lock" {
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }

  # You can also specify multiple extra arguments for each use case. Here we configure terragrunt to always pass in the
  # `common.tfvars` var file located by the parent terragrunt config.
  extra_arguments "custom_vars" {
    commands = [
      "apply",
      "plan",
      "import",
      "push",
      "refresh"
    ]

    required_var_files = ["${get_parent_terragrunt_dir()}/common.tfvars"]
  }

  # The following are examples of how to specify hooks

  # Before apply or plan, run "echo Foo".
  before_hook "before_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Foo"]
  }

  # Before apply, run "echo Bar". Note that blocks are ordered, so this hook will run after the previous hook to
  # "echo Foo". In this case, always "echo Bar" even if the previous hook failed.
  before_hook "before_hook_2" {
    commands     = ["apply"]
    execute      = ["echo", "Bar"]
    run_on_error = true
  }

  # Note that you can use interpolations in subblocks. Here, we configure it so that before apply or plan, print out the
  # environment variable "HOME".
  before_hook "interpolation_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", get_env("HOME", "HelloWorld")]
    run_on_error = false
  }

  # After running apply or plan, run "echo Baz". This hook is configured so that it will always run, even if the apply
  # or plan failed.
  after_hook "after_hook_1" {
    commands     = ["apply", "plan"]
    execute      = ["echo", "Baz"]
    run_on_error = true
  }

  # After an error occurs during apply or plan, run "echo Error Hook executed". This hook is configured so that it will run
  # after any error, with the ".*" expression.
  error_hook "error_hook_1" {
    commands  = ["apply", "plan"]
    execute   = ["echo", "Error Hook executed"]
    on_errors = [
      ".*",
    ]
  }

  # A special after hook to always run after the init-from-module step of the Terragrunt pipeline. In this case, we will
  # copy the "foo.tf" file located by the parent terragrunt.hcl file to the current working directory.
  after_hook "init_from_module" {
    commands = ["init-from-module"]
    execute  = ["cp", "${get_parent_terragrunt_dir()}/foo.tf", "."]
  }

  # A special after_hook. Use this hook if you wish to run commands immediately after terragrunt finishes loading its
  # configurations. If "terragrunt-read-config" is defined as a before_hook, it will be ignored as this config would
  # not be loaded before the action is done.
  after_hook "terragrunt-read-config" {
    commands = ["terragrunt-read-config"]
    execute  = ["bash", "script/get_aws_credentials.sh"]
  }
}
```

Local File Path Example with allowed hidden files:

```hcl
terraform {
  # Pull the terraform configuration from the local file system. Terragrunt will make a copy of the source folder in the
  # Terragrunt working directory (typically `.terragrunt-cache`).
  source = "../modules/networking/vpc"

  # Always include the following file patterns in the Terragrunt copy.
  include_in_copy = [
    ".security_group_rules.json",
    "*.yaml",
  ]
}
```


#### A note about using modules from the registry

The key design of Terragrunt is to act as a preprocessor to convert **shared service modules** in the registry into a **root
module**. In Terraform, modules can be loosely categorized into two types:

* **Root Module**: A Terraform module that is designed for running `terraform init` and the other workflow commands
  (`apply`, `plan`, etc). This is the entrypoint module for deploying your infrastructure. Root modules are identified
  by the presence of key blocks that setup configuration about how Terraform behaves, like `backend` blocks (for
  configuring state) and `provider` blocks (for configuring how Terraform interacts with the cloud APIs).
* **Shared Module**: A Terraform module that is designed to be included in other Terraform modules through `module`
  blocks. These modules are missing many of the key blocks that are required for running the workflow commands of
  terraform.

Terragrunt further distinguishes shared modules between **service modules** and **modules**:

* **Shared Service Module**: A Terraform module that is designed to be standalone and applied directly. These modules
  are not root modules in that they are still missing the key blocks like `backend` and `provider`, but aside from that
  do not need any additional configuration or composition to deploy. For example, the
  [terraform-aws-modules/vpc](https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest) module can be
  deployed by itself without composing with other modules or resources.
* **Shared Module**: A Terraform module that is designed to be composed with other modules. That is, these modules must
  be embedded in another Terraform module and combined with other resources or modules. For example, the
  [consul-security-group-rules
  module](https://registry.terraform.io/modules/hashicorp/consul/aws/latest/submodules/consul-security-group-rules)

Terragrunt started off with features that help directly deploy **Root Modules**, but over the years have implemented
many features that allow you to turn **Shared Service Modules** into **Root Modules**  by injecting the key configuration
blocks that are necessary for Terraform modules to act as **Root Modules**.

Modules on the Terraform Registry are primarily designed to be used as **Shared Modules**. That is, you won't be able to
`git clone` the underlying repository and run `terraform init` or `apply` directly on the module without modification.
Unless otherwise specified, almost all the modules will require composition with other modules/resources to deploy.
When using modules in the registry, it helps to think about what blocks and resources are necessary to operate the
module, and translating those into Terragrunt blocks that generate them.

Note that in many cases, Terragrunt may not be able to deploy modules from the registry. While Terragrunt has features
to turn any **Shared Module** into a **Root Module**, there are two key technical limitations that prevent Terragrunt
from converting ALL shared modules:

- Every complex input must have a `type` associated with it. Otherwise, Terraform will interpret the input that
  Terragrunt passes through as `string`. This includes `list` and `map`.
- Derived sensitive outputs must be marked as `sensitive`. Refer to the [terraform tutorial on sensitive
  variables](https://learn.hashicorp.com/tutorials/terraform/sensitive-variables#reference-sensitive-variables) for more
  information on this requirement.

**If you run into issues deploying a module from the registry, chances are that module is not a Shared Service Module,
and thus not designed for use with Terragrunt. Depending on the technical limitation, Terragrunt may be able to
support the transition to root module. Please always file [an issue on the terragrunt
repository](https://github.com/gruntwork-io/terragrunt/issues) with the module + error message you are encountering,
instead of the module repository.**



### remote_state

The `remote_state` block is used to configure how Terragrunt will set up the remote state configuration of your
Terraform code. You can read more about Terragrunt's remote state functionality in [Keep your remote state configuration
DRY](/docs/features/keep-your-remote-state-configuration-dry/) use case overview.

The `remote_state` block supports the following arguments:

- `backend` (attribute): Specifies which remote state backend will be configured. This should be one of the
  [backend types](https://www.terraform.io/docs/backends/types/index.html) that Terraform supports.

- `disable_init` (attribute): When `true`, skip automatic initialization of the backend by Terragrunt. Some backends
  have support in Terragrunt to be automatically created if the storage does not exist. Currently `s3` and `gcs` are the
  two backends with support for automatic creation. Defaults to `false`.

- `disable_dependency_optimization` (attribute): When `true`, disable optimized dependency fetching for terragrunt
  modules using this `remote_state` block. See the documentation for [dependency block](#dependency) for more details.

- `generate` (attribute): Configure Terragrunt to automatically generate a `.tf` file that configures the remote state
  backend. This is a map that expects two properties:
    - `path`: The path where the generated file should be written. If a relative path, it'll be relative to the Terragrunt
      working dir (where the terraform code lives).
    - `if_exists` (attribute): What to do if a file already exists at `path`. Valid values are: `overwrite` (overwrite the
      existing file), `overwrite_terragrunt` (overwrite the existing file if it was generated by terragrunt; otherwise,
      error) `skip` (skip code generation and leave the existing file as-is), `error` (exit with an error).

- `config` (attribute): An arbitrary map that is used to fill in the backend configuration in Terraform. All the
  properties will automatically be included in the Terraform backend block (with a few exceptions: see below). For
  example, if you had the following `remote_state` block:

    ```hcl
    remote_state {
      backend = "s3"
      config = {
        bucket = "mybucket"
        key    = "path/to/my/key"
        region = "us-east-1"
      }
    }
    ```

  This is equivalent to the following `terraform` code:

    ```hcl
    terraform {
      backend "s3" {
        bucket = "mybucket"
        key    = "path/to/my/key"
        region = "us-east-1"
      }
    }
    ```

Note that `remote_state` can also be set as an attribute. This is useful if you want to set `remote_state` dynamically.
For example, if in `common.hcl` you had:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket = "mybucket"
    key    = "path/to/my/key"
    region = "us-east-1"
  }
}
```

Then in a `terragrunt.hcl` file, you could dynamically set `remote_state` as an attribute as follows:

```hcl
locals {
  # Load the data from common.hcl
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

# Set the remote_state config dynamically to the remote_state config in common.hcl
remote_state = local.common.remote_state
```

Note that Terragrunt does special processing of the `config` attribute for the `s3` and `gcs` remote state backends, and
supports additional keys that are used to configure the automatic initialization feature of Terragrunt.

For the `s3` backend, the following additional properties are supported in the `config` attribute:

- `region` - (Optional) The region of the S3 bucket.
- `profile` - (Optional) This is the AWS profile name as set in the shared credentials file.
- `endpoint` - (Optional) A custom endpoint for the S3 API.
- `encrypt` - (Optional) Whether to enable server side encryption of the state file.
- `role_arn` - (Optional) The role to be assumed.
- `shared_credentials_file` - (Optional) This is the path to the shared credentials file. If this is not set and a profile is specified, `~/.aws/credentials` will be used.
- `external_id` - (Optional) The external ID to use when assuming the role.
- `session_name` - (Optional) The session name to use when assuming the role.
- `dynamodb_table` - (Optional) The name of a DynamoDB table to use for state locking and consistency. The table must have a primary key named LockID. If not present, locking will be disabled.
- `skip_bucket_versioning`: When `true`, the S3 bucket that is created to store the state will not be versioned.
- `skip_bucket_ssencryption`: When `true`, the S3 bucket that is created to store the state will not be configured with server-side encryption.
- `skip_bucket_accesslogging`: _DEPRECATED_ If provided, will be ignored. A log warning will be issued in the console output to notify the user.
- `skip_bucket_root_access`: When `true`, the S3 bucket that is created will not be configured with bucket policies that allow access to the root AWS user.
- `skip_bucket_enforced_tls`: When `true`, the S3 bucket that is created will not be configured with a bucket policy that enforces access to the bucket via a TLS connection.
- `skip_bucket_public_access_blocking`: When `true`, the S3 bucket that is created will not have public access blocking enabled.
- `disable_bucket_update`: When `true`, disable update S3 bucket if not equal configured in config block
- `enable_lock_table_ssencryption`: When `true`, the synchronization lock table in DynamoDB used for remote state concurrent access will not be configured with server side encryption.
- `s3_bucket_tags`: A map of key value pairs to associate as tags on the created S3 bucket.
- `dynamodb_table_tags`: A map of key value pairs to associate as tags on the created DynamoDB remote state lock table.
- `accesslogging_bucket_tags`: A map of key value pairs to associate as tags on the created S3 bucket to store de access logs.
- `disable_aws_client_checksums`: When `true`, disable computing and checking checksums on the request and response,
  such as the CRC32 check for DynamoDB. This can be used to workaround
  https://github.com/gruntwork-io/terragrunt/issues/1059.
- `accesslogging_bucket_name`: (Optional) When provided as a valid `string`, create an S3 bucket with this name to store the access logs for the S3 bucket used to store Terraform state. If not provided, or string is empty or invalid S3 bucket name, then server access logging for the S3 bucket storing the terraform state will be disabled. **Note:** When access logging is enabled supported encryption for state bucket is only `AES256`. Reference: [S3 server access logging](https://docs.aws.amazon.com/AmazonS3/latest/userguide/enable-server-access-logging.html)
- `accesslogging_target_prefix`: (Optional) When provided as a valid `string`, set the `TargetPrefix` for the access log objects in the S3 bucket used to store Terraform state. If set to **empty**`string`, then `TargetPrefix` will be set to **empty** `string`. If attribute is not provided at all, then `TargetPrefix` will be set to **default** value `TFStateLogs/`. This attribute won't take effect if the `accesslogging_bucket_name` attribute is not present.
- `bucket_sse_algorithm`: (Optional) The algorithm to use for server side encryption of the state bucket. Defaults to `aws:kms`.
- `bucket_sse_kms_key_id`: (Optional) The KMS Key to use when the encryption algorithm is `aws:kms`. Defaults to the AWS Managed `aws/s3` key.

For the `gcs` backend, the following additional properties are supported in the `config` attribute:

- `skip_bucket_creation`: When `true`, Terragrunt will skip the auto initialization routine for setting up the GCS
  bucket for use with remote state.
- `skip_bucket_versioning`: When `true`, the GCS bucket that is created to store the state will not be versioned.
- `enable_bucket_policy_only`: When `true`, the GCS bucket that is created to store the state will be configured to use uniform bucket-level access.
- `project`: The GCP project where the bucket will be created.
- `location`: The GCP location where the bucket will be created.
- `gcs_bucket_labels`: A map of key value pairs to associate as labels on the created GCS bucket.
- `credentials`: Local path to Google Cloud Platform account credentials in JSON format.
- `access_token`: A temporary [OAuth 2.0 access token] obtained from the Google Authorization server.
Example with S3:

```hcl
# Configure terraform state to be stored in S3, in the bucket "my-terraform-state" in us-east-1 under a key that is
# relative to included terragrunt config. For example, if you had the following folder structure:
#
# .
# ├── terragrunt.hcl
# └── child
#     └── terragrunt.hcl
#
# And the following is defined in the root terragrunt.hcl config that is included in the child, the state file for the
# child module will be stored at the key "child/terraform.tfstate".
#
# Note that since we are not using any of the skip args, this will automatically create the S3 bucket
# "my-terraform-state" and DynamoDB table "my-lock-table" if it does not already exist.
remote_state {
  backend = "s3"
  config = {
    bucket         = "my-terraform-state"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

Example with GCS:

```hcl
# Configure terraform state to be stored in GCS, in the bucket "my-terraform-state" in the "my-terraform" GCP project in
# the eu region under a key that is relative to included terragrunt config. This will also apply the labels
# "owner=terragrunt_test" and "name=terraform_state_storage" to the bucket if it is created by Terragrunt.
#
# For example, if you had the following folder structure:
#
# .
# ├── terragrunt.hcl
# └── child
#     └── terragrunt.hcl
#
# And the following is defined in the root terragrunt.hcl config that is included in the child, the state file for the
# child module will be stored at the key "child/terraform.tfstate".
#
# Note that since we are not using any of the skip args, this will automatically create the GCS bucket
# "my-terraform-state" if it does not already exist.
remote_state {
  backend = "gcs"

  config = {
    project  = "my-terraform"
    location = "eu"
    bucket   = "my-terraform-state"
    prefix   = "${path_relative_to_include()}/terraform.tfstate"

    gcs_bucket_labels = {
      owner = "terragrunt_test"
      name  = "terraform_state_storage"
    }
  }
}
```



### include

The `include` block is used to specify inheritance of Terragrunt configuration files. The included config (also called
the `parent`) will be merged with the current configuration (also called the `child`) before processing. You can learn
more about the inheritance properties of Terragrunt in the [Filling in remote state settings with Terragrunt
section](/docs/features/keep-your-remote-state-configuration-dry/#filling-in-remote-state-settings-with-terragrunt) of the
"Keep your remote state configuration DRY" use case overview.

You can have more than one `include` block, but each one must have a unique label. It is recommended to always label
your `include` blocks. Bare includes (`include` block with no label - e.g., `include {}`) are currently supported for
backward compatibility, but is deprecated usage and support may be removed in the future.

`include` blocks support the following arguments:

- `name` (label): You can define multiple `include` blocks in a single terragrunt config. Each include block
  must be labeled with a unique name to differentiate it from the other includes. E.g., if you had a block `include
  "remote" {}`, you can reference the relevant exposed data with the expression `include.remote`.
- `path` (attribute): Specifies the path to a Terragrunt configuration file (the `parent` config) that should be merged
  with this configuration (the `child` config).
- `expose` (attribute, optional): Specifies whether or not the included config should be parsed and exposed as a
  variable. When `true`, you can reference the data of the included config under the variable `include`. Defaults to
  `false`. Note that the `include` variable is a map of `include` labels to the parsed configuration value.
- `merge_strategy` (attribute, optional): Specifies how the included config should be merged. Valid values are:
  `no_merge` (do not merge the included config), `shallow` (do a shallow merge - default), `deep` (do a deep merge of
  the included config).

**NOTE**: At this time, Terragrunt only supports a single level of `include` blocks. That is, Terragrunt will error out
if an included config also has an `include` block defined. If you are interested in this feature, please follow
https://github.com/gruntwork-io/terragrunt/issues/1566 to be notified when nested `include` blocks are supported.

**Special case for shallow merge**: When performing a shallow merge, all attributes and blocks are merged shallowly with
replacement, except for `dependencies` blocks (NOT `dependency` block). `dependencies` blocks are deep merged: that is,
all the lists of paths from included configurations are concatenated together, rather than replaced in override fashion.


Examples:

_Single include_
```hcl
# If you have the following folder structure, and the following contents for ./child/terragrunt.hcl, this will include
# and merge the items in the terragrunt.hcl file at the root.
#
# .
# ├── terragrunt.hcl
# └── child
#     └── terragrunt.hcl
include "root" {
  path   = find_in_parent_folders()
  expose = true
}

inputs = {
  remote_state_config = include.root.remote_state
}
```

_Multiple includes_
```hcl
# If you have the following folder structure, and the following contents for ./child/terragrunt.hcl, this will include
# and merge the items in the terragrunt.hcl file at the root, while only loading the data in the region.hcl
# configuration.
#
# .
# ├── terragrunt.hcl
# ├── region.hcl
# └── child
#     └── terragrunt.hcl
include "remote_state" {
  path   = find_in_parent_folders()
  expose = true
}

include "region" {
  path           = find_in_parent_folders("region.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

inputs = {
  remote_state_config = include.remote_state.remote_state
  region              = include.region.region
}
```

**Limitations on accessing exposed config**

In general, you can access all attributes on `include` when they are exposed (e.g., `include.locals`, `include.inputs`,
etc).

However, to support `run-all`, Terragrunt is unable to expose all attributes when the included config has a `dependency`
block. To understand this, consider the following example:

_root terragrunt.hcl_
```hcl
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
}

inputs = {
  vpc_name = dependency.vpc.outputs.name
}
```

_child terragrunt.hcl_
```hcl
include "root" {
  path   = find_in_parent_folders()
  expose = true
}

dependency "alb" {
  config_path = (
    include.root.inputs.vpc_name == "mgmt"
    ? "../alb-public"
    : "../alb-private"
  )
}

input = {
  alb_id = dependency.alb.outputs.id
}
```

In the child `terragrunt.hcl`, the `dependency` path for the `alb` depends on whether the VPC is the `mgmt` VPC or not,
which is determined by the `dependency.vpc` in the root config. This means that the output from `dependency.vpc` must be
available to parse the `dependency.alb` config.

This causes problems when performing a `run-all apply` operation. During a `run-all` operation, Terragrunt first parses
all the `dependency` blocks to build a dependency tree of the Terragrunt modules to figure out the order of operations.
If all the paths are static references, then Terragrunt can determine all the dependency paths before any module has
been applied. In this case there is no problem even if other config blocks access `dependency`, as by the time
Terragrunt needs to parse those blocks, the upstream dependencies would have been applied during the `run-all apply`.

However, if those `dependency` blocks depend on upstream dependencies, then there is a problem as Terragrunt would not
be able to build the dependency tree without the upstream dependencies being applied.

Therefore, to ensure that Terragrunt can build the dependency tree in a `run-all` operation, Terragrunt enforces the
following limitation to exposed `include` config:

If the included configuration has any `dependency` blocks, only `locals` and `include` are exposed and available to the
child `include` and `dependency` blocks. There are no restrictions for other blocks in the child config (e.g., you can
reference `inputs` from the included config in child `inputs`).

Otherwise, if the included config has no `dependency` blocks, there is no restriction on which exposed attributes you
can access.

For example, the following alternative configuration is valid even if the alb dependency is still accessing the `inputs`
attribute from the included config:

_root terragrunt.hcl_
```hcl
inputs = {
  vpc_name = "mgmt"
}
```

_child terragrunt.hcl_
```hcl
include "root" {
  path   = find_in_parent_folders()
  expose = true
}

dependency "vpc" {
  config_path = "../vpc"
}

dependency "alb" {
  config_path = (
    include.root.inputs.vpc_name == "mgmt"
    ? "../alb-public"
    : "../alb-private"
  )
}

input = {
  vpc_name = dependency.vpc.outputs.name
  alb_id   = dependency.alb.outputs.id
}
```



**What is deep merge?**

When the `merge_strategy` for the `include` block is set to `deep`, Terragrunt will perform a deep merge of the included
config. For Terragrunt config, deep merge is defined as follows:

- For simple types, the child overrides the parent.
- For lists, the two attribute lists are combined together in concatenation.
- For maps, the two maps are combined together recursively. That is, if the map keys overlap, then a deep merge is
  performed on the map value.
- For blocks, if the label is the same, the two blocks are combined together recursively. Otherwise, the blocks are
  appended like a list. This is similar to maps, with block labels treated as keys.

However, due to internal implementation details, some blocks are not deep mergeable. This will change in the future, but
for now, terragrunt performs a shallow merge (that is, block definitions in the child completely override the parent
definition). The following blocks have this limitation:
    - `remote_state`
    - `generate`

Similarly, the `locals` block is deliberately omitted from the merge operation by design. That is, you will not be able
to access parent config `locals` in the child config, and vice versa in a merge. However, you can access the parent
locals in child config if you use the `expose` feature.

Finally, `dependency` blocks have special treatment. When doing a `deep` merge, `dependency` blocks from **both** child
and parent config are accessible in **both** places. For example, consider the following setup:

_parent config_
```hcl
dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  db_id = dependency.mysql.outputs.db_id
}
```

_child config_
```hcl
include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}

dependency "mysql" {
  config_path = "../mysql"
}

inputs = {
  security_group_id = dependency.vpc.outputs.security_group_id
}
```

In the example, note how the parent is accessing the outputs of the `mysql` dependency even though it is not defined in
the parent. Similarly, the child is accessing the outputs of the `vpc` dependency even though it is not defined in the
child.

Full example:

_parent terragrunt.hcl_
```hcl
remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "${path_relative_to_include()}/terraform.tfstate"
    region = "us-west-2"
  }
}

dependency "vpc" {
  # This will get overridden by child terragrunt.hcl configs
  config_path = ""

  mock_outputs = {
    attribute     = "hello"
    old_attribute = "old val"
    list_attr     = ["hello"]
    map_attr = {
      foo = "bar"
    }
  }
  mock_outputs_allowed_terraform_commands = ["apply", "plan", "destroy", "output"]
}

inputs = {
  attribute     = "hello"
  old_attribute = "old val"
  list_attr     = ["hello"]
  map_attr = {
    foo = "bar"
    test = dependency.vpc.outputs.new_attribute
  }
}
```

_child terragrunt.hcl_
```hcl
include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}

remote_state {
  backend = "local"
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    attribute     = "mock"
    new_attribute = "new val"
    list_attr     = ["mock"]
    map_attr = {
      bar = "baz"
    }
  }
}

inputs = {
  attribute     = "mock"
  new_attribute = "new val"
  list_attr     = ["mock"]
  map_attr = {
    bar = "baz"
  }

  dep_out = dependency.vpc.outputs
}
```

_merged terragrunt.hcl_
```hcl
# Child override parent completely due to deep merge limitation
remote_state {
  backend = "local"
}

# mock_outputs are merged together with deep merge
dependency "vpc" {
  config_path = "../vpc"       # Child overrides parent
  mock_outputs = {
    attribute     = "mock"     # Child overrides parent
    old_attribute = "old val"  # From parent
    new_attribute = "new val"  # From child
    list_attr     = [
      "hello",                 # From parent
      "mock",                  # From child
    ]
    map_attr = {
      foo = "bar"              # From parent
      bar = "baz"              # From child
    }
  }

  # From parent
  mock_outputs_allowed_terraform_commands = ["apply", "plan", "destroy", "output"]
}

# inputs are merged together with deep merge
inputs = {
  attribute     = "mock"       # Child overrides parent
  old_attribute = "old val"    # From parent
  new_attribute = "new val"    # From child
  list_attr     = [
    "hello",                 # From parent
    "mock",                  # From child
  ]
  map_attr = {
    foo = "bar"                                   # From parent
    bar = "baz"                                   # From child
    test = dependency.vpc.outputs.new_attribute   # From parent, referencing dependency mock output from child
  }

  dep_out = dependency.vpc.outputs                # From child
}
```


### locals

The `locals` block is used to define aliases for Terragrunt expressions that can be referenced within the configuration.
You can learn more about locals in the [feature overview](/docs/features/locals/).

The `locals` block does not have a defined set of arguments that are supported. Instead, all the arguments passed into
`locals` are available under the reference `local.ARG_NAME` throughout the Terragrunt configuration.

Example:

```hcl
# Make the AWS region a reusable variable within the configuration
locals {
  aws_region = "us-east-1"
}

inputs = {
  region = local.aws_region
  name   = "${local.aws_region}-bucket"
}
```


### dependency

The `dependency` block is used to configure module dependencies. Each dependency block exports the outputs of the target
module as block attributes you can reference throughout the configuration. You can learn more about `dependency` blocks
in the [Dependencies between modules
section](/docs/features/execute-terraform-commands-on-multiple-modules-at-once/#dependencies-between-modules) of the
"Execute Terraform commands on multiple modules at once" use case overview.

You can define more than one `dependency` block. Each label you provide to the block identifies another `dependency`
that you can reference in your config.

The `dependency` block supports the following arguments:

- `name` (label): You can define multiple `dependency` blocks in a single terragrunt config. As such, each block needs a
  name to differentiate between the other blocks, which is what the first label of the block is used for. You can
  reference the specific dependency output by the name. E.g if you had a block `dependency "vpc"`, you can reference the
  outputs of this dependency with the expression `dependency.vpc.outputs`.
- `config_path` (attribute): Path to a Terragrunt module (folder with a `terragrunt.hcl` file) that should be included
  as a dependency in this configuration.
- `skip_outputs` (attribute): When `true`, skip calling `terragrunt output` when processing this dependency. If
  `mock_outputs` is configured, set `outputs` to the value of `mock_outputs`. Otherwise, `outputs` will be set to an
  empty map. Put another way, setting `skip_outputs` means "use mocks all the time if `mock_outputs` are set."
- `mock_outputs` (attribute): A map of arbitrary key value pairs to use as the `outputs` attribute when no outputs are
  available from the target module, or if `skip_outputs` is `true`. However, it's generally recommended not to set
  `skip_outputs` if using `mock_outputs`, because `skip_outputs` means "use mocks all the time if they are set" whereas
  `mock_outputs` means "use mocks only if real outputs are not available." Use `locals` instead when `skip_outputs = true`.
- `mock_outputs_allowed_terraform_commands` (attribute): A list of Terraform commands for which `mock_outputs` are
  allowed. If a command is used where `mock_outputs` is not allowed, and no outputs are available in the target module,
  Terragrunt will throw an error when processing this dependency.
- `mock_outputs_merge_with_state` (attribute): DEPRECATED. Use `mock_outputs_merge_strategy_with_state`. When `true`,
  `mock_outputs` and the state outputs will be merged. That is, the `mock_outputs` will be treated as defaults and the
  real state outputs will overwrite them if the keys clash.
- `mock_outputs_merge_strategy_with_state` (attribute): Specifies how any existing state should be merged into the
  mocks. Valid values are
  - `no_merge` (default) - any existing state will be used as is. If the dependency does not have an existing state (it
    hasn't been applied yet), then the mocks will be used
  - `shallow` - the existing state will be shallow merged into the mocks. Mocks will only be used where the output does
    not already exist in the dependency's state
  - `deep_map_only` - the existing state will be deeply merged into the mocks. If an output is a map, the mock key
    will be used where that key does not exist in the state. Lists will not be merged

Example:

```hcl
# Run `terragrunt output` on the module at the relative path `../vpc` and expose them under the attribute
# `dependency.vpc.outputs`
dependency "vpc" {
  config_path = "../vpc"

  # Configure mock outputs for the `validate` command that are returned when there are no outputs available (e.g the
  # module hasn't been applied yet.
  mock_outputs_allowed_terraform_commands = ["validate"]
  mock_outputs = {
    vpc_id = "fake-vpc-id"
  }
}

# Another dependency, available under the attribute `dependency.rds.outputs`
dependency "rds" {
  config_path = "../rds"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  db_url = dependency.rds.outputs.db_url
}
```

**Can I speed up dependency fetching?**

`dependency` blocks are fetched in parallel at each source level, but will serially parse each recursive dependency. For
example, consider the following chain of dependencies:

```
account --> vpc --> securitygroup --> ecs
                                      ^
                                     /
                              ecr --
```

In this chain, the `ecr` and `securitygroup` module outputs will be fetched concurrently when applying the `ecs` module,
but the outputs for `account` and `vpc` will be fetched serially as terragrunt needs to recursively walk through the
tree to retrieve the outputs at each level.

This recursive parsing happens due to the necessity to parse the entire `terragrunt.hcl` configuration (including
`dependency` blocks) in full before being able to call `terraform output`.

However, terragrunt includes an optimization to only fetch the lowest level outputs (`securitygroup` and `ecr` in this
example) provided that the following conditions are met in the immediate dependencies:

- The remote state is managed using `remote_state` blocks.
- The dependency optimization feature flag is enabled (`disable_dependency_optimization = false`, which is the default).
- The `remote_state` block itself does not depend on any `dependency` outputs (`locals` and `include` are ok).
- You are not relying on `before_hook`, `after_hook`, or `extra_arguments` to the `terraform init` call. NOTE:
  terragrunt will not automatically detect this and you will need to explicitly opt out of the dependency optimization
  flag.

If these conditions are met, terragrunt will only parse out the `remote_state` blocks and use that to pull down the
state for the target module without parsing the `dependency` blocks, avoiding the recursive dependency retrieval.


### dependencies

The `dependencies` block is used to enumerate all the Terragrunt modules that need to be applied in order for this
module to be able to apply. Note that this is purely for ordering the operations when using `run-all` commands of
Terraform. This does not expose or pull in the outputs like `dependency` blocks.

The `dependencies` block supports the following arguments:

- `paths` (attribute): A list of paths to modules that should be marked as a dependency.

Example:

```hcl
# When applying this terragrunt config in an `run-all` command, make sure the modules at "../vpc" and "../rds" are
# handled first.
dependencies {
  paths = ["../vpc", "../rds"]
}
```


### generate

The `generate` block can be used to arbitrarily generate a file in the terragrunt working directory (where `terraform`
is called). This can be used to generate common terraform configurations that are shared across multiple terraform
modules. For example, you can use `generate` to generate the provider blocks in a consistent fashion by defining a
`generate` block in the parent terragrunt config.

The `generate` block supports the following arguments:

- `name` (label): You can define multiple `generate` blocks in a single terragrunt config. As such, each block needs a
  name to differentiate between the other blocks.
- `path` (attribute): The path where the generated file should be written. If a relative path, it'll be relative to the
  Terragrunt working dir (where the terraform code lives).
- `if_exists` (attribute): What to do if a file already exists at `path`. Valid values are: `overwrite` (overwrite the
  existing file), `overwrite_terragrunt` (overwrite the existing file if it was generated by terragrunt; otherwise,
  error) `skip` (skip code generation and leave the existing file as-is), `error` (exit with an error).
- `comment_prefix` (attribute): A prefix that can be used to indicate comments in the generated file. This is used by
  terragrunt to write out a signature for knowing which files were generated by terragrunt. Defaults to `# `. Optional.
- `disable_signature` (attribute): When `true`, disables including a signature in the generated file. This means that
  there will be no difference between `overwrite_terragrunt` and `overwrite` for the `if_exists` setting. Defaults to
  `false`. Optional.
- `contents` (attribute): The contents of the generated file.
- `disable` (attribute): Disables this generate block.

Example:

```hcl
# When using this terragrunt config, terragrunt will generate the file "provider.tf" with the aws provider block before
# calling to terraform. Note that this will overwrite the `provider.tf` file if it already exists.
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents = <<EOF
provider "aws" {
  region              = "us-east-1"
  version             = "= 2.3.1"
  allowed_account_ids = ["1234567890"]
}
EOF
}
```

Note that `generate` can also be set as an attribute. This is useful if you want to set `generate` dynamically.
For example, if in `common.hcl` you had:

```hcl
  generate "provider" {
    path      = "provider.tf"
    if_exists = "overwrite"
    contents = <<EOF
provider "aws" {
  region              = "us-east-1"
  version             = "= 2.3.1"
  allowed_account_ids = ["1234567890"]
}
EOF
}
```

Then in a `terragrunt.hcl` file, you could dynamically set `generate` as an attribute as follows:

```hcl
locals {
  # Load the data from common.hcl
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

# Set the generate config dynamically to the generate config in common.hcl
generate = local.common.generate
```

## Attributes

- [inputs](#inputs)
- [download_dir](#download_dir)
- [prevent_destroy](#prevent_destroy)
- [skip](#skip)
- [iam_role](#iam_role)
- [iam_assume_role_duration](#iam_assume_role_duration)
- [iam_assume_role_session_name](#iam_assume_role_session_name)
- [terraform_binary](#terraform_binary)
- [terraform_version_constraint](#terraform_version_constraint)
- [terragrunt_version_constraint](#terragrunt_version_constraint)
- [retryable_errors](#retryable_errors)


### inputs

The `inputs` attribute is a map that is used to specify the input variables and their values to pass in to Terraform.
Each entry of the map will be passed to Terraform using [the environment variable
mechanism](https://www.terraform.io/docs/configuration/variables.html#environment-variables). This means that each input
will be set using the form `TF_VAR_variablename`, with the value in `json` encoded format.

Note that because the values are being passed in with environment variables and `json`, the type information is lost
when crossing the boundary between Terragrunt and Terraform. You must specify the proper [type
constraint](https://www.terraform.io/docs/configuration/variables.html#type-constraints) on the variable in Terraform in
order for Terraform to process the inputs to the right type.

Example:

```hcl
inputs = {
  string      = "string"
  number      = 42
  bool        = true
  list_string = ["a", "b", "c"]
  list_number = [1, 2, 3]
  list_bool   = [true, false]

  map_string = {
    foo = "bar"
  }

  map_number = {
    foo = 42
    bar = 12345
  }

  map_bool = {
    foo = true
    bar = false
    baz = true
  }

  object = {
    str  = "string"
    num  = 42
    list = [1, 2, 3]

    map = {
      foo = "bar"
    }
  }

  from_env = get_env("FROM_ENV", "default")
}
```


### download_dir

The terragrunt `download_dir` string option can be used to override the default download directory.

The precedence is as follows: `--terragrunt-download-dir` command line option → `TERRAGRUNT_DOWNLOAD` env variable →
`download_dir` attribute of the `terragrunt.hcl` file in the module directory → `download_dir` attribute of the included
`terragrunt.hcl`.

It supports all terragrunt functions, i.e. `path_relative_from_include()`.


### prevent_destroy

Terragrunt `prevent_destroy` boolean flag allows you to protect selected Terraform module. It will prevent `destroy` or
`destroy-all` command to actually destroy resources of the protected module. This is useful for modules you want to
carefully protect, such as a database, or a module that provides auth.

Example:

```hcl
terraform {
  source = "git::git@github.com:foo/modules.git//app?ref=v0.0.3"
}

prevent_destroy = true
```

### skip

The terragrunt `skip` boolean flag can be used to protect modules you don’t want any changes to or just to skip modules
that don’t define any infrastructure by themselves. When set to true, all terragrunt commands will skip the selected
module.

Consider the following file structure:

    root
    ├── terragrunt.hcl
    ├── prod
    │   └── terragrunt.hcl
    ├── dev
    │   └── terragrunt.hcl
    └── qa
        └── terragrunt.hcl

In some cases, the root level `terragrunt.hcl` file is solely used to DRY up your Terraform configuration by being
included in the other `terragrunt.hcl` files. In this case, you do not want the `run-all` commands to process the root
level `terragrunt.hcl` since it does not define any infrastructure by itself. To make the `run-all` commands skip the
root level `terragrunt.hcl` file, you can set `skip = true`:

``` hcl
skip = true
```

The `skip` flag must be set explicitly in terragrunt modules that should be skipped. If you set `skip = true` in a
`terragrunt.hcl` file that is included by another `terragrunt.hcl` file, only the `terragrunt.hcl` file that explicitly
set `skip = true` will be skipped.


### iam_role

The `iam_role` attribute can be used to specify an IAM role that Terragrunt should assume prior to invoking Terraform.

The precedence is as follows: `--terragrunt-iam-role` command line option → `TERRAGRUNT_IAM_ROLE` env variable →
`iam_role` attribute of the `terragrunt.hcl` file in the module directory → `iam_role` attribute of the included
`terragrunt.hcl`.

Example:

```hcl
iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
```
**Notes:**
  * Value of `iam_role` can reference local variables
  * Definitions of `iam_role` included from other HCL files through `include`

### iam_assume_role_duration

The `iam_assume_role_duration` attribute can be used to specify the STS session duration, in seconds, for the IAM role that Terragrunt should assume prior to invoking Terraform.

The precedence is as follows: `--terragrunt-iam-assume-role-duration` command line option → `TERRAGRUNT_IAM_ASSUME_ROLE_DURATION` env variable →
`iam_assume_role_duration` attribute of the `terragrunt.hcl` file in the module directory → `iam_assume_role_duration` attribute of the included
`terragrunt.hcl`.

Example:

```hcl
iam_assume_role_duration = 14400
```

### iam_assume_role_session_name

The `iam_assume_role_session_name` attribute can be used to specify the STS session name, for the IAM role that Terragrunt should assume prior to invoking Terraform.

The precedence is as follows: `--terragrunt-iam-assume-role-session-name` command line option → `TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME` env variable →
`iam_assume_role_session_name` attribute of the `terragrunt.hcl` file in the module directory → `iam_assume_role_session_name` attribute of the included
`terragrunt.hcl`.


### terraform_binary

The terragrunt `terraform_binary` string option can be used to override the default terraform binary path (which is
`terraform`).

The precedence is as follows: `--terragrunt-tfpath` command line option → `TERRAGRUNT_TFPATH` env variable →
`terragrunt.hcl` in the module directory → included `terragrunt.hcl`


### terraform_version_constraint

The terragrunt `terraform_version_constraint` string overrides the default minimum supported version of terraform.
Terragrunt only officially supports the latest version of terraform, however in some cases an old terraform is needed.

Example:

```hcl
terraform_version_constraint = ">= 0.11"
```

### terragrunt_version_constraint

The terragrunt `terragrunt_version_constraint` string can be used to specify which versions of the Terragrunt CLI can be used with your configuration. If the running version of Terragrunt doesn't match the constraints specified, Terragrunt will produce an error and exit without taking any further actions.

Example:

```hcl
terragrunt_version_constraint = ">= 0.23"
```

### retryable_errors

The terragrunt `retryable_errors` list can be used to override the default list of retryable errors with your own custom list.
To learn more about the `retryable_errors` attribute, see the [auto-retry feature overview](/docs/features/auto-retry).

Example:

```hcl
retryable_errors = [
  "(?s).*Error installing provider.*tcp.*connection reset by peer.*",
  "(?s).*ssh_exchange_identification.*Connection closed by remote host.*"
]
```
