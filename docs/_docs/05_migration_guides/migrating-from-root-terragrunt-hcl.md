---
layout: collection-browser-doc
title: Migrating from root `terragrunt.hcl`
category: migrate
categories_url: migrate
excerpt: Migration guide to replace root `terragrunt.hcl` file
tags: ["migration", "community"]
order: 501
nav_title: Documentation
nav_title_link: /docs/
---

## Problem

Recommended best practices for Terragrunt used to be that users would defined two `terragrunt.hcl` files for any significantly large code base:

1. A root `terragrunt.hcl` file that defined the Terragrunt configuration common to all units in the code base.
2. Child `terragrunt.hcl` files that defined the Terragrunt configuration specific to each [unit](/docs/getting-started/terminology/#unit) in the code base.

This was a simple pattern that made it very obvious what these files were used for, and certain Terragrunt features (like `find_in_parent_folders`) assumed this default structure.

Over time, this has caused confusion for users of Terragrunt, however. See [#3181](https://github.com/gruntwork-io/terragrunt/issues/3181) for an example of the confusion this has caused.

At the end of the day, from a functional perspective, it doesn't actually help users to have the root configuration named `terragrunt.hcl`. It makes it more confusing to determine what is shared configuration and what is configuration for a unit.

It also complicates Terragrunt usage, as commands like `run-all` need to be run from a directory where all child directories will be `terragrunt.hcl` files corresponding to units that need to be run.

## Recommended Solution

To simplify Terragrunt usage and make it more clear what the root configuration is, we recommend that users rename the root `terragrunt.hcl` file to something else (e.g. `root.hcl`).

This will simplify Terragrunt usage, as you will no longer need to carefully avoid running a Terragrunt command in a way that it might think the root `terragrunt.hcl` file is a unit configuration, and it will make it more obvious what is and isn't a unit.

Note that in addition to renaming the root `terragrunt.hcl` file, you will also need to update any Terragrunt configurations that assume the root configuration will be named `terragrunt.hcl`. The most common example of this would be usage of `find_in_parent_folders` without any arguments. By default, this will look for a `terragrunt.hcl` file, so you will need to update this to look for the new root configuration file name.

e.g.

```hcl
# /some/path/terragrunt.hcl
include {
  path = find_in_parent_folders()
}
```

To:

```hcl
# /some/path/terragrunt.hcl
include {
  path = find_in_parent_folders("root.hcl")
}
```

## Additional Considerations

If you use [Scaffold](/docs/features/scaffold) and [Catalog](/docs/features/catalog), you may need to use additional flags to control how new units are generated. It was previously a safe assumption that most users would leverage a root `terragrunt.hcl` file, and thus, the default behavior was to generate a new unit that would look for a `terragrunt.hcl` file in the root.

You can use the `--no-include-root` and `--root-file-name` flags of both commands to explicitly control how new units are generated, and what they will look for as the root configuration file (or if they should look for one at all).

e.g.

```bash
terragrunt catalog
```

To:

```bash
terragrunt catalog --root-file-name root.hcl
```

## Strict Control

To enforce this newly recommended pattern, you can also enable the [root-terragrunt-hcl](/docs/reference/strict-mode/#root-terragrunt-hcl) strict control to throw an error when Terragrunt detects that a root `terragrunt.hcl` file is being used.

e.g.

```bash
terragrunt plan
```

To:

```bash
terragrunt plan --strict-control=root-terragrunt-hcl
```

Or:

```bash
TERRAGRUNT_STRICT_CONTROL=root-terragrunt-hcl terragrunt plan
```

By enabling the strict control, you will also have the default behavior of `scaffold` and `catalog` commands changed to use `root.hcl` as the default root configuration file name if none are provided.

## Future Behavior

For now, warnings will be emitted when this pattern is detected in order to encourage users to change to the new pattern, but this behavior will be an explicit error in a future version of Terragrunt. Given how long this has been the standard pattern, we want to assure users that they will have a _very_ long time to migrate to this new pattern. For the most part, using the old pattern results in very little practical difference in Terragrunt behavior, assuming Terragrunt usage is already working fine.
