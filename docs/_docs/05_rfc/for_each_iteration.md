---
layout: collection-browser-doc
title: for_each to call terraform module multiple times
category: RFC
categories_url: rfc
excerpt: for_each - looping variables to call module multiple times.
tags: ["rfc", "contributing", "community"]
order: 502
nav_title: Documentation
nav_title_link: /docs/
---

# RFC: for_each - looping variables to call module multiple times

**STATUS: Won't Implement**


## Background

Oftentimes it is desirable to repeatedly apply a module multiple times with only a few modifications. This is common
when you have a canonical way to provision resources and need to quickly produce copies with one or two modifications.
For example, in Kubernetes, you might have a Namespace factory module that provisions Namespaces in a canonical way that
you configure with a common set of variables, and the only thing that differs across multiple Namespaces is the name.

Another use case is in achieving compliance with the CIS benchmark for AWS. One of the requirements in the benchmark is
that you need to enable AWS Config in all enabled regions on your account. In this case, you would want to loop over
each enabled region and run the module that provisions and configures AWS Config using the same args.

Ideally, we can handle this in the Terraform module by looping over `module` blocks:

```
module "aws_config" {
  for_each = var.all_enabled_regions
  region   = each.key

  # additional arguments omitted for brevity
}
```

However, [as of 0.12.7, this is still not available](https://github.com/hashicorp/terraform/issues/17519). That said,
this is being developed and there is reason to believe that this will eventually be available, especially since,
starting with Terraform 0.12.0, `count` and `for_each` has been reserved on `module` blocks. What is not known is how
long it will take before `for_each` is implemented on modules.

There are a few advantages to a Terragrunt implementation of `for_each` to call modules repeatedly with different
parameters:

- Provide a workaround sooner than Terraform might implement module `for_each` and `count`.
- You can keep separate state files for each module call. While this does not help with isolation of blast radius (due
  to the looping interface), this helps with isolating access to individual state files. For example, you can adjust the
  remote state config such that the bucket is parameterized by the looping argument, allowing you to store the state of
  each module call in a different S3 bucket.
- You can allow separate IAM roles for each loop iteration. This can be done if you parameterize the `iam_role`
  property with the looping argument.

For this reason, it is worth contemplating a possible implementation path for supporting iteration in Terragrunt to
repeatedly call a module with different parameters. Note when considering an implementation, it is important to optimize
for **speed of implementation and simplicity**, since it is very likely that this feature will be made obsolete by
`for_each` on `module` blocks.


## Proposed solution

We decided to go with Option 1 where we will not implement any new features in Terragrunt to specifically handle this
use case, given the complexity of the task involved and the relative short lifespan of such a feature. If you have a
need for replicating modules, we recommend using a code generation tool to templatize your Terraform modules.


## Alternatives

### Option 1: Null option - do nothing

Since we already know Terraform wants to support looping `module` blocks, we can make the conclusion to wait for
Terraform to implement it as opposed to implementing a workaround in Terragrunt.

**Pros**:

- Keeps Terragrunt clean, since no new code is introduced.
- Relies on official solution. We don't want to introduce workarounds in Terragrunt unless it is absolutely necessary
  and unlikely to be supported officially in Terraform.

**Cons**:

- We must wait for Hashicorp to implement the feature. Given how the resources, data sources, and providers work in
  `module` blocks, looping modules is a reasonably complex feature, both in terms of state representation and resource
  management. As such, this is expected to take some time, as described in [the 0.12
  preview](https://www.hashicorp.com/blog/hashicorp-terraform-0-12-preview-for-and-for-each#module-count-and-for_each).

### Option 2: for_each attribute in terragrunt config that "generates" multiple copies of the config

##### Description

In this approach, looping is handled at the top level using a new attribute named `for_each` to mimic the `for_each`
construct in Terraform. For example:

```
# terragrunt.hcl
for_each = ["us-east-1", "us-west-1"]

inputs = {
  region = each.value
}
```

Internally, this will translate to multiple copies of `config.TerragruntConfig` structs, one for each item in the
`for_each` collection with `each.value` interpolated to the value for that item. Then, Terragrunt will run the command
on each config in a loop.

One challenge with this approach is that the state needs to be unique for each item in the `for_each` collection to
avoid conflict in the state file. Since each call is a separate module call, having each one share state will result in
each call stepping over each other. Therefore, it is important that this implementation **add a guard to ensure the
remote state file is unique per iteration**.

To support this, there should be a way to partially patch the `remote_state` config that is inherited from a parent. For
example, we should be able to do the following:

_Parent terragrunt.hcl_

```
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

_Child terragrunt.hcl_

```
for_each = ["us-east-1", "us-west-1"]

remote_state {
  # Added for backwards compatibility. This should default to false, so that existing config overrides completely as
  # opposed to partially.
  merge_parent = true

  config = {
    key = "dev/${each.value}/_global/aws-config/terraform.tfstate"
  }
}

inputs = {
  region = each.value
}
```

In this setup, the `remote_state` block in the child `terragrunt.hcl` should be used to patch the parent `remote_state`
block, as opposed to completely replacing it as it is now in the current behavior. This way, you can still keep your
`remote_state` config DRY while only updating the relevant pieces to support iteration.

##### Implementation detail

To implement this, the following changes need to be made:

- `ParseConfigFile`, `ParseConfigString`, `PartialParseConfigFile`, and `PartialParseConfigString` need to be updated to
  return list of `TerragruntConfig` structs, as opposed to a single one.
- Each caller of parsing functions need to accordingly handle the list of `TerragruntConfig` objects. As a part of this,
  the caller should loop through each config to run the commands in the context of each iteration of the expanded loop.
- The terragrunt workspace logic needs to be updated to ensure a separate module folder is created for each loop
  iteration.
- The parsing functions need to be updated to first parse the `for_each` attribute. Then, the parser should iterate each
  item in the list and set the `each` variable accordingly as it parses the rest of the config. This means the parsing
  order is now adjusted as:
    - `for_each` and expand loop.
    - `locals`
    - `include`
    - `dependency`
    - Everything else
- Support `merge_parent` in `remote_state`, which ultimately adjusts how `mergeConfigWithIncludedConfig` merges the
  child `remote_state` config with the parent.


##### Pros and Cons of approach

**Pros**:

- Parallels `for_each` in Terraform. In the examples I used `list`, but the implementation details can mimic that of
  Terraform (requiring `set` or `map`). That way, the syntax closely resembles an existing construct and thus it will be
  easier to pick up.
- Limited changes are necessary in the user config to add in iteration.
- The config parser implementation is relatively simple.

**Cons**:

- The implementation of the runners can become complex, especially in the dependency detection for the `xxx-all`
  commands. If `dependency` blocks are parameterized by looping, this can be hairy to unpack and expand into the
  dependency graph if the terragrunt files don't exist.
- The logic for expanding out the loop for the terragrunt cache is complex as it is not immediately obvious what the
  folder structure should be. Should it be a hash of the iteration value appended to the working path (e.g
  `.terragrunt-cache/HASH_OF_EACH_VALUE/path/to/workspace`)? Or should it be one cache per iteration value (e.g
  `HASH_OF_EACH_VALUE/.terragrunt-cache/path/to/workspace`)?
- Implementation requires a major refactor in the calling logic post parsing.
- It is not immediately obvious how to handle removing an item in the `for_each` list. A naive approach would not run
  `terragrunt destroy` on that module. Additionally, as stated, it is impossible to run the command on a single item in
  the list and thus you can't destroy just the items you want to remove without doing complex dancing (e.g update the
  `for_each` list to only reference the items you want to destroy, then run `terragrunt destroy` to destroy just those
  items, and then restore the list to the version with the items removed).

### Option 3: scaffolding tool that code gens live config using a template

Instead of implementing looping within the terragrunt execution logic, this approach proposes generating the live config
through templating. The idea here would be to use a templating tool (e.g
[cookiecutter](https://github.com/cookiecutter/cookiecutter)) to render each live config parameterized by some list
value that will then generate the folder structure.

In this approach, no change is necessary within Terragrunt to support this other than perhaps documentation and examples
to demonstrate the approach.

**Pros**:

- Keeps Terragrunt clean, since no new code is introduced.
- Available workaround now, without any work.

**Cons**:

- Code generation is complex.
    - When should you run the code generation step?
    - How often?
    - How do you know if there is drift?
    - Do you check in the generated code? If so, you have to maintain it (e.g., 19 sets of files/folders for 19 AWS
      regions). If it's always generated at runtime, then interacting with just one of the generated things becomes
      harder.
- Removing an iteration still requires a bit of a dance. You need to `terragrunt destroy` the relevant folder, then
  remove that folder, and then remove the corresponding item from the iteration list.
- Users now need to learn a templating tool and language, on top of Terragrunt and Terraform.
- It is not immediately obvious where these templates should live. Is it a part of `infrastructure-modules`?
  `infrastructure-live`? Or a completely separate repo?


## References

- [Terraform Issue proposing for_each on modules](https://github.com/hashicorp/terraform/issues/17519)
- [Original RFC PR](https://github.com/gruntwork-io/terragrunt/pull/853)
