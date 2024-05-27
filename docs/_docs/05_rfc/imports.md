---
layout: collection-browser-doc
title: Imports
category: RFC
categories_url: rfc
excerpt: Define new mechanisms for importing terragrunt config.
tags: ["rfc", "contributing", "community"]
order: 504
nav_title: Documentation
nav_title_link: /docs/
---

<!-- markdownlint-disable -->

# Imports

**STATUS**: In development


## Background

### Problem

Currently, Terragrunt does not provide a lot of flexibility when it comes to reusing values from other configs.
The only feature available to users right now for config reuse is the `include` block, which allows you to include and
merge all the values from another `terragrunt.hcl` config. The `include` block has a few limitations:

- You do not have fine grained control over what values get merged. The entire config has to be inherited.
- You can only include and inherit one level.
- You have limited ability to affect the parent config from the child config.

The canonical use case where these limitations get in the way is if you want to construct a hierarchy of inputs that get
merged. For example, consider the following canonical terragrunt folder structure:

```
prod
└── us-east-1
    └── app
        └── vpc
            └── terragrunt.hcl
```

As you progress down the directory, there is a desire to automatically include variables that specify the encapsulated
environment such that you can predict the target environment based on where the config lies in the folder structure. For
example, when you are in the `us-east-1` folder, you will want to pass in the variable `aws_region = "us-east-1"` to
terraform. Similarly, you will want to set `vpc_name = "app"` when you are in the `app` folder of a particular region,
so that you deploy to that particular VPC.

In Terragrunt 0.18 and Terraform 0.11, this was accomplished by specifying `tfvars` files in the folder hierarchy:

```
prod
├── us-east-1
│   ├── app
│   │   ├── env.tfvars
│   │   └── vpc
│   │       └── terraform.tfvars
│   └── region.tfvars
└── terraform.tfvars
```

You would then include each `tfvars` in the hierarchy in the root `terraform.tfvars` file using the `extra_arguments`
setting with `optional_var_files`.

While you can still implement the same mechanism with Terragrunt 0.19 and above, a change in behavior in Terraform 0.12
made it so that you must have all the variables that are being included specified in the child modules. This means that
even if you had a module that did not depend on the AWS region (e.g a Kubernetes Service), you have to specify the
`aws_region` variable in order for this to work.

Another limitation of this approach is if the region variable is under a different name in the module being deployed.
This assumes that all the variable names for the region must be the same, which is oftentimes not the case, especially
when you want to deploy third party modules as well. In this case, you want to specify the various permutations in the
`region.tfvars` file but you won't be able to do that because you will run into the limitation where no module will have
all the permutations defined as variables.

The current workaround for this use case is to specify the variables in `json` or `yaml` and merge them into the
`inputs` attribute of the root `terragrunt.hcl` file using the `jsondecode` / `yamldecode` function with `merge`. While
this works, the configuration becomes fairly verbose as you try to workaround the fact that not all directories will
have all the yaml files in the hierarchy. See [this example config](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example/blob/b796c371f631b9ba42189fef744601cdd16d48f5/non-prod/terragrunt.hcl#L30).

Another limitation of `yaml` and `json` is that they are static configurations. This means that you can't share
complex variables in the middle of the hierarchy that might require more computation than hard coded values. For
example, suppose that you wanted to always include the `vpc_id` when you are at the `app` level of the folder hierarchy.
Ideally, you would be able to specify:

```hcl
dependency "vpc" {
  config_path = "/path/to/app/vpc/module"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

and then auto merge this configuration with the children. However, since we can't rely on terragrunt parsing for the
middle layers, and since we can only include one level of terragrunt configuration, there is no way to do the above
without introducing the dependency to all configurations, which is not what you want.


### Goals

Given the problem statement, this RFC aims to propose a solution that addresses the following:

- Provide a way to reuse partial terragrunt configurations.
- Provide a way to reuse from multiple terragrunt configuration files.
- Maintain design principles of explicit vs. implicit. Avoid confusing behavior that adds cognitive load, such as monkey
  patching logic.
- Avoid verbosity where possible.
- Avoid expanding the number of constructs in terragrunt (e.g a helper function or block). If a new construct has to be
  introduced, it should deprecate and replace an existing construct.


## Proposed solution

The proposed solution is an incremental improvement to the situation by implementing a series of increasingly more
expensive solutions. The following is a summary of the solutions to be built:

- [Short term: read_terragrunt_config helper function](#read_terragrunt_config-helper-function)
- [Medium term: import blocks](#import-blocks)
- [Long term: single terragrunt.hcl file](#single-terragrunthcl-file-per-environment)

### read_terragrunt_config helper function

For this approach, we define a helper function that parses the relevant config and exposes it for use in the terragrunt
config. For example, the explicit triple import example in the [hierarchical variables use
case](#hierarchical-variables-included-across-multiple-terragrunt-hcl-files) can be implemented as:

```hcl
locals {
  root_config = read_terragrunt_config("../../../root.hcl")
  region_config = read_terragrunt_config("../../region.hcl")
  env_config = read_terragrunt_config("../env.hcl")
}

inputs = merge(
  local.root_config.inputs,
  local.region_config.inputs,
  local.env_config.inputs,
  {
    # args to module
  },
)
```

Pros:

- Relatively simple implementation. It is significantly easier to add helper functions than it is to introduce new
  blocks in terragrunt.
- Supports all the use cases described above.

Cons:

- Everything has to be explicit in the config. We can not support automatic merging capabilities when using helper
  functions, as you can't manipulate the config in a helper. This can lead to very verbose configurations if one wants
  to use this in place of `include`.

Note that to take full advantage of this approach, all the blocks in `terragrunt.hcl` need to be redefined as attributes
so that we can use assignment to override them.

Let's walk through a few more of the use cases:

#### (read_terragrunt_config) Keeping remote state configuration DRY

parent

```hcl
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

child

```hcl
locals {
  root_config = read_terragrunt_config("../../../root.hcl")
}

remote_state {
  backend = local.root_config.remote_state.backend
  config = merge(
    local.root_config.remote_state.config,
    {
      # The relative path between the root terragrunt directory and the current terragrunt file directory.
      key =  relpath(import.root.terragrunt_dir, get_terragrunt_dir())
    },
  )
}
```

Note that we can't do:

```hcl
remote_state = deep_merge(local.root_config.remote_state, { config = { key = relpath } })
```

due to the fact that `remote_state` is a block and not an attribute.


#### (read_terragrunt_config) Reusing dependencies

We can't reuse `dependency` blocks in this implementation because there is no way to auto merge the blocks.

If `dependency` was instead an attribute, we could use the following alternative syntax:

vpc_dependency_config.hcl

```hcl
dependency = {
  vpc = {
    config_path = "/path/to/app/vpc/module"
  }
}
```

```hcl
locals {
  common_deps = read_terragrunt_config("../vpc_dependency_config.hcl")
}

dependency = local.common_deps.dependency

inputs = {
  name = "unique-name"
  vpc_id = dependency.vpc.outputs.vpc_id
}
```


### import blocks

This approach is to introduce a new block `import` which replaces the functionality of `include`. We use a new
block instead of reusing `include` for backwards compatibility. As part of the implementation, `include` and the
relevant functions (`get_parent_terragrunt_dir`, `path_relative_to_include`, and `path_relative_from_include`) will be
deprecated and will throw a warning whenever someone uses it.

The `import` block works as follows:

- When the `import` block appears in a terragrunt config, the target config is parsed in full before the current config.
- Subsequent blocks that are parsed will be given the context of the `import` and any block and attribute of the
  imported config can be referenced.
- Complex structures like `remote_state` will have their properties nested. E.g you can reference the backend of the
  imported config under the name `remote_state.backend`.
- Blocks that support multiple declarations (e.g `dependency`) will be referenced by name. E.g if you had in the parent:

      dependency "vpc" {}
      dependency "db" {}

  You can reference each dependency block in the child as `dependency.vpc` and `dependency.db` respectively.

- Imports are one way: you can not have circular imports, and you can not influence the parent config to affect other
  children (commonly known as monkey patching).
- You can chain imports. As in, imported configs can themselves import other config, as long as the chain of imports
  does not form a cycle.
- Imports support auto merging via the `merge` setting. When `merge = true`, all the blocks and attributes of the
  imported config will be merged with the child config. Note that since HCL blocks are sequential, imports will be
  merged top to bottom. See below for more details.
- Imports can also be deep merged using the `deep_merge` setting. This will work similar to `merge = true`.
- Imports will also export the absolute path to the directory of the terragrunt config file as the attribute
  `terragrunt_dir`.
- Imports are parsed first in the [configuration parsing
  order](https://terragrunt.gruntwork.io/docs/getting-started/configuration/#configuration-parsing-order). This is to
  allow references to the imports in `locals`. This is because it is more likely that one would want to break down
  imports attributes into local references than want to use locals in the `import` block given that it only has a single
  attribute pointing to the target config.
- Imports are not compatible with `include`. Having both blocks will cause a terragrunt syntax error.

Pros:

- Supports all the use cases described above.

Cons:

- Since we don't allow `path_relative` functions, reusability is limited when compared to `include` (e.g the remote
  state use case). This can be resolved if we implement the relevant `path_relative` functions for `import` blocks.


Let's take a look at a few common use cases and how we might use `import` to address them:

- [Hierarchical variables included across multiple terragrunt.hcl
  files](#import-block-hierarchical-variables-included-across-multiple-terragrunthcl-files)
- [Reusing common variables](#import-block-reusing-common-variables)
- [Reusing dependencies](#import-block-reusing-dependencies)
- [Keeping remote state configuration DRY](#import-block-keeping-remote-state-configuration-dry)

#### (import block) Hierarchical variables included across multiple terragrunt.hcl files

Consider the following folder structure from the canonical example:

```
prod
├── us-east-1
│   ├── app
│   │   ├── env.hcl
│   │   └── vpc
│   │       └── terragrunt.hcl
│   └── region.hcl
└── account.hcl
```

With `import` blocks, you can implement the automatic variable merging in the following manner:

prod/account.hcl

```hcl
inputs = {
  account_id = 0000000
}
```

prod/us-east-1/region.hcl

```hcl
import "account" {
  config_path = "../account.hcl"
}

inputs = merge(
  import.account.inputs,
  {
    region = "us-east-1"
  },
)
```

prod/us-east-1/app/env.hcl

```hcl
import "region" {
  config_path = "../region.hcl"
}

inputs = merge(
  import.region.inputs,
  {
    env = "prod"
  },
)
```

prod/us-east-1/app/vpc/terragrunt.hcl

```hcl
import "env" {
  config_path = "../env.hcl"
}

inputs = merge(
  import.env.inputs,
  {
    # args to module
  },
)
```

Note how there is a chain of imports that add additional inputs as we progress down the hierarchy. Each level appends
additional inputs that are made available for use to deeper levels of the hierarchy. This means that by
the time we get to the leaf (`prod/us-east-1/app/vpc`), all the required inputs will have been merged in. The nice thing
about this behavior is that everything you need to know about the configuration is explicitly mentioned. There is no
implicit values that change based on who is importing the configuration.

Here is another alternative that avoids deep imports:

prod/account.hcl

```hcl
inputs = {
  account_id = 0000000
}
```

prod/us-east-1/region.hcl

```hcl
inputs = {
  region = "us-east-1"
}
```

prod/us-east-1/app/env.hcl

```hcl
inputs = {
  env = "prod"
}
```

prod/us-east-1/app/vpc/terragrunt.hcl

```hcl
import "root" {
  config_path = "../../../root.hcl"
  merge = true
}

import "region" {
  config_path = "../../region.hcl"
  merge = true
}

import "env" {
  config_path = "../env.hcl"
  merge = true
}

inputs = {
  # args to module
}
```

This trades off verbosity in the child config with a relatively simpler import path, where there is only one level of
imports. Note how all three imports have `merge = true`. This is equivalent to the following:

```
import "root" {
  config_path = "../../../root.hcl"
}

import "region" {
  config_path = "../../region.hcl"
}

import "env" {
  config_path = "../env.hcl"
}

inputs = merge(
  import.root.inputs,
  import.region.inputs,
  import.env.inputs,
  {
    # args to module
  },
)
```

The `merge = true` option is a convenience feature to make the child config less verbose in case you have multiple
overridable configuration in your hierarchy.


#### (import block) Reusing common variables

Many resources are named with the region or environment that they belong to. A canonical terragrunt live configuration
uses hierarchical variables to pass in the region and environment settings to terraform. However, there is no way in the
current implementation to compose those variables to construct different inputs (e.g a `name` variable that includes the
`region` variable). You had to rely on doing the composition in terraform. This works if you have control over the
module, but sometimes you want to directly deploy a third party module (e.g from the registry) and it seems heavy to
have to fork or wrap that module just for the name.

With `import` blocks, you can implement this by referencing the `region` input from the `import`:

region config
```
import "region" {
  config_path = "../../region.hcl"
}

inputs = merge(
  import.region.inputs,
  {
    name = "${import.region.inputs["region"]}-unique-name"
  },
)
```


#### (import block) Reusing dependencies

In the problem statement we discussed a use case where we want to pass in and merge the `vpc_id`. This can be
implemented with `import` blocks in two ways.

- [Auto merge](#auto-merge)
- [Explicit reference](#explicit-reference)

##### Auto merge

With auto merge, you can `import` the configuration that declares the dependency and the input directly and have nothing
in the child config. For example:

vpc_dependency_config.hcl

```
dependency "vpc" {
  config_path = "/path/to/app/vpc/module"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

```
import "vpc_dependency_config" {
  config_path = "../vpc_dependency_config.hcl"
  merge = true
}

inputs = {
  name = "unique-name"
}
```

This will pass in the input variables `vpc_id` and `name` to the terraform configuration, where the `vpc_id` input comes
from the `vpc_dependency_config.hcl` configuration import that is merged in. Note that we can add in additional
configurations by adding another `import` block. For example, if you had a root config that specifies remote state
configurations, you can add another `import` block for it to pull it in:

```
import "root" {
  config_path = "../../root.hcl"
  merge = true
}

import "vpc_dependency_config" {
  config_path = "../vpc_dependency_config.hcl"
  merge = true
}

inputs = {
  name = "unique-name"
}
```

##### Explicit reference

As an alternative to auto merge, you can also be explicit in referencing the inputs block in the import:

```
import "vpc_dependency_config" {
  config_path = "../vpc_dependency_config.hcl"
}

inputs = merge(
  import.vpc_dependency_config.inputs,
  {
    name = "unique-name"
  },
)
```

Or, if you needed to pass in the under a different name (e.g `id_of_vpc`), you can access the vpc dependency across the
import:

```
import "vpc_dependency_config" {
  config_path = "../vpc_dependency_config.hcl"
}

inputs = {
  name = "unique-name"
  id_of_vpc = import.vpc_dependency_config.dependency.vpc.outputs.vpc_id
}
```



#### (import block) Keeping remote state configuration DRY

A canonical use case for terragrunt is to share the remote state configuration. The classic example is having a root
`terragrunt.hcl` configuration that specifies the `remote_state`, which is then imported using `include` to all the
other configurations:

parent

```hcl
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

child

```hcl
include {
  path = find_in_parent_folders()
}
```

In this world, the parent configuration (in this case the `remote_state` block) is automatically "merged" into the child
configuration (NOTE: this is not an actual merge as it does not do a deep merge. If the child had a `remote_state`
block, the entire block is replaced.).

A key feature here is the use of the `path_relative_to_include` to monkey patch the S3 key of the parent config based on
who is importing. For example:

```
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
```

Here, the S3 key of the statefile for each of the child configurations will be as follows:

```
backend-app => backend-app/terraform.tfstate
frontend-app => frontend-app/terraform.tfstate
mysql => mysql/terraform.tfstate
vpc => vpc/terraform.tfstate
```

This is because the relative path from the parent config to the child config is the single level of folders in the
hierarchy.

However, this can be confusing as now there is complex cognitive load on the reader to know the relative path between
the two configs to fully know what the key should be. This can be difficult to do in your head since you do not have the
paths in view in the config!

With `import` blocks, we can implement this in the following way:

parent

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = "my-terraform-state"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "my-lock-table"
    # `key` must be set by the child configs.
  }
}
```

child

```hcl
import "root" {
  config_path = find_in_parent_folders("root.hcl")
  deep_merge = true
}

remote_state {
  config = {
    # The relative path between the root terragrunt directory and the current terragrunt file directory.
    key =  relpath(import.root.terragrunt_dir, get_terragrunt_dir())
  }
}
```

Note how we explicitly set the relative path using only the context of the current file and the parent. There is no
circular reference here where you need the context of the child to know the exact values of the attributes being set in
the parent (although you won't know the full setting of `remote_state` without seeing the child).

Additionally, here we take advantage of the `deep_merge` feature to deep merge the two configurations for
`remote_state`.  This is equivalent to having the following `remote_state` block in the child config:

```hcl
remote_state {
  backend = "s3"
  config = {
    bucket         = "my-terraform-state"
    region         = "us-east-1"
    # The relative path between the root terragrunt directory and the current terragrunt file directory.
    key            =  relpath("/abspath/to/dir/containing/root.hcl", get_terragrunt_dir())
    encrypt        = true
    dynamodb_table = "my-lock-table"
  }
}
```

If you wanted to be explicit and avoid the implicit `merge` of the blocks, you can also explicitly set the attributes of
the block in the child config:

```hcl
import "root" {
  config_path = find_in_parent_folders("root.hcl")
}

remote_state {
  backend = import.root.remote_state.backend
  config  = merge(
    import.root.remote_state.config,
    {
      # The relative path between the root terragrunt directory and the current terragrunt file directory.
      key =  relpath(import.root.terragrunt_dir, get_terragrunt_dir())
    },
  )
}
```

NOTE: `relpath` does not currently exist, and must be implemented as a terragrunt helper function.

The advantage of this configuration is that we're only pulling the `remote_state` configuration to the child. The other
blocks and attributes (e.g `inputs`) is not inherited to the child config. This was something that was not possible to
do with `include`.

##### Why can't we use the merge function?

In the above example, we have to explicitly declare the block. A natural extension of this pattern is to use the `merge`
function directly on the `remote_state`:

```hcl
remote_state = merge(
  import.root.remote_state,
  {
    config = {
      key = relpath(import.root.terragrunt_dir, get_terragrunt_dir())
    }
  }
)
```

Unfortunately, this is a syntax error because `remote_state` is a block, and not an attribute. You can not directly set
blocks in this way.


### Single terragrunt.hcl file per environment

This implementation introduces a new block `module` that replaces `dependency`, `terraform`, and `inputs`. This approach
is documented in [#759](https://github.com/gruntwork-io/terragrunt/issues/759).

In addition to the general analysis of that proposal, here are the list of Pros and Cons related to the problem of
sharing config:

Pros:

- Circumvent the reference problem by having all the references in a single shared scope.

Cons:

- If you need to share configurations across environments, you would still need one of the alternative approaches to
  import the other config. This, however, is likely to be a rare occurrence.
- This is a drastic change to Terragrunt, completely changing the way it works.


Let's walk through how each of the import use cases look like with this implementation:

#### (single file) Hierarchical variables

In this approach, a hierarchy of variables is unnecessary because all the blocks are defined in a single scope. However,
depending on the scope of `locals`, certain things like reusing a repetitive variable becomes more challenging.

To understand this, consider a use case where you are operating in two regions, `us-east-1` and `eu-west-1`. To simplify
this example, consider a limited application deployment where you have two modules: `vpc` and `app`.

If we assume that `locals` are scoped within the file that they are defined, then you can implement this by separating
the environment `terragrunt.hcl` into two files, `us_east_1.hcl` and `eu_west_1.hcl`:

us_east_1.hcl

```hcl
locals {
  region = "us-east-1"
}

module "vpc" {
  # additional args omitted for brevity
  aws_region = local.region
}

module "app" {
  # additional args omitted for brevity
  aws_region = local.region
}
```

eu_west_1.hcl

```hcl
locals {
  region = "eu-west-1"
}

module "vpc" {
  # additional args omitted for brevity
  aws_region = local.region
}

module "app" {
  # additional args omitted for brevity
  aws_region = local.region
}
```

However, if the namespace of `locals` was shared across the environment, then the above approach would not work and each
region file will need to define a different name for the region variable.

Note that in this example, we assumed that the environment split is at the account level. An alternative split is to
have each environment be at the region level. The advantage of this approach is to define a different state bucket for
each region, which is a better posture for disaster recovery. In this approach, it doesn't matter what the scope of the
`locals` is. However, the downside of this approach is that there will be some repetition to pull in the AWS account ID
info across all the different regions.

#### (single file) Reusing common variables

Reusing common variables depends on the scope of `locals`. If the scope of `locals` is shared across the environment,
then you can define the common variable in `locals` blocks to share across the entier environment. If instead the scope
of `locals` is per file, we either:

- Define all the code for the single environment in a single file.
- Implement [globals](#globals) in a way that can be shared across the environment.

_Reusing dependencies_

Reusing dependencies is not a problem in this approach because the namespace for the environment is shared. That is, you
can reference any of the other `module` blocks to hook up the dependency within a single environment. For example, if
you had two modules `app` and `mysql` which depend on the `vpc` module, you could define the config as follows:

```hcl
module "vpc" {
  # args omitted for brevity
}

module "app" {
  # args omitted for brevity
  vpc_id = module.vpc.outputs.vpc_id
}

module "mysql" {
  # args omitted for brevity
  vpc_id = module.vpc.outputs.vpc_id
}
```

This example reuses the outputs of `module.vpc` across the two modules, which is the equivalent of having the `vpc`
`dependency` block redefined in the two module configs.

#### (single file) Keeping remote state configuration DRY

This example is covered in [the original issue that proposed this
idea](https://github.com/gruntwork-io/terragrunt/issues/759).




## Alternatives

### Enhancing include with import semantics

Instead of having a dedicated block with the new functionality, we could enhance `include` to have all the semantics of
`import`. This reduces complexity of the configuration by being able to recycle a very similar construct that already
exists.

However, this means that we are locked into supporting all the functions that allow manipulation of the parent
configuration based on who has included it (e.g `path_relative_to_include`). Users will be used to and expect continued
support for such semantics, and possibly request feature enhancements that further encourage monkey patching.

Additionally, the backwards compatibility story of reusing the include block can lead to confusion. Reusing `include`
implies that we should default `merge` to `true` to maintain backwards compatibility. Defaulting to `false` not only
breaks existing configurations, but it breaks them in a very subtle way where the parent configuration is not merged
into the child. This can be especially problematic to a user who is only relying on `include` to keep their remote state
configuration DRY, as all their remote state configuration will be missing and a naive `apply-all` for a new environment
may not surface this fact because it will "just work" on the local state. Note that this is a realistic scenario:
imagine a new person on your team who installs the latest version of terragrunt with this change, but the team has been
using the older version without this update. They then start to add a new component, and when they run plan, everything
will look correct because they are adding new resources, but they may not realize that it is not storing to remote
state!

To summarize:

Pros:

- Reduce configuration complexity by reusing an existing block.
- Avoid confusion from having two ways to do similar things (`import` with `merge = true` vs. `include`).

Cons:

- Potentially complex implementation due to having to touch and modify lots of existing code.
- Locks us into supporting existing semantics around "monkey patching"
- Backwards incompatibility has gotchas that can cause frustrations to existing teams.

Ultimately, the potential for badly shooting yourself in the foot was enough to warrant a new block to start clean.


### globals

Globals was a potential solution to the problem proposed by the community. `globals` work the same way as `locals`,
except they support merging across `include`. For example, to address the use case of [reusing common
variables](#reusing-common-variables), you could have the following:

parent config

```
globals {
  aws_region = "us-east-1"
}
```

child config

```
include {
  path = find_in_parent_folders()
}

inputs = {
  aws_region = global.aws_region
  name = "${global.aws_region}-unique-name"
}
```

Note how the child config accesses the `global` variable that is defined in the parent config, without having defined
the `globals` block.

`globals` also had the ability to "monkey patch" the parent config. For example:

parent config

```
globals {
  aws_region = "us-east-1"
}

inputs = {
  aws_region = global.aws_region
  name = "${global.aws_region}-${global.name_suffix}"
}
```

child config

```
globals {
  name_suffix = "unique-name"
}

include {
  path = find_in_parent_folders()
}
```

Note how the child config updated the `global` variable in the parent config by specifying a `globals` block.

You can read more about the proposal in [the issue](https://github.com/gruntwork-io/terragrunt/issues/814) and
[corresponding PR](https://github.com/gruntwork-io/terragrunt/pull/858).

Pros:

- Reuses existing import mechanism.
- Powerful construct that can be used to simulate custom functions in terragrunt configs by monkey patching.

Cons:

- Encourages monkey patching, which generally increases complexity and deteriorates readability/maintainability.
- Implementation can be complex due to graph logic between globals and locals to allow reuse across the two.
- Does not solve all the problems in this RFC. E.g `globals` does not address the need for multiple includes and
  fine grained control over merging.

Ultimately, the complexity of the implementation and the monkey patching behavior suggested that it may be better to
look for an alternative implementation.



## Open Question

- While a major design goal in this RFC is to keep the principles of explicit over implicit, practically speaking it may
  not be feasible to achieve that design goal to support all the use cases of the community. In that regard, it may be
  useful to implement a minimal set of features that don't take a whole lot of cognitive load to understand. Should the
  following functions be implemented for use with `import`?
    - `path_relative_to_import` and `path_relative_from_import`: These are the equivalent functions of the ones named
      for `include`.
    - `find_in_parent_folders_from_importing_config`: This function is the version of `find_in_parent_folders` that
      works in the context of the config that is importing the current config.


## References

This challenge has come up numerous times in the lifetime of Terragrunt. The following are relevant issues that raise
similar concerns:

- [Shared and overridable variabls](https://github.com/gruntwork-io/terragrunt/issues/814)
- [Being able to merge maps from inputs](https://github.com/gruntwork-io/terragrunt/issues/744)
- [Request to allow more than one level of include](https://github.com/gruntwork-io/terragrunt/issues/303)
- [Request to reference inputs from another config](https://github.com/gruntwork-io/terragrunt/issues/967)
- [Partially override components of an input](https://github.com/gruntwork-io/terragrunt/issues/1011)

Relevant PRs and Releases:

- [PR for RFC](https://github.com/gruntwork-io/terragrunt/pull/1025)
- [PR for read_terragrunt_config](https://github.com/gruntwork-io/terragrunt/pull/1051) (released in
  [v0.22.3](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.22.3))
