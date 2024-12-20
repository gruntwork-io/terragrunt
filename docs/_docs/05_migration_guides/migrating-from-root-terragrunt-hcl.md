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

The recommended best practice for Terragrunt usage was previously that users defined two types of `terragrunt.hcl` files for any significantly large code base:

1. A root `terragrunt.hcl` file that defined the Terragrunt configuration common to all units in the code base.
2. Child `terragrunt.hcl` files that defined the Terragrunt configuration specific to each [unit](/docs/getting-started/terminology/#unit) in the code base.

This was a simple pattern that made it very obvious what these files were used for (Terragrunt), and certain Terragrunt features (like `find_in_parent_folders`) assumed this default structure.

Over time, this caused confusion for users of Terragrunt, however. See [#3181](https://github.com/gruntwork-io/terragrunt/issues/3181) for an example of the confusion this has caused.

At the end of the day, from a functional perspective, it doesn't actually help users to have the root configuration named `terragrunt.hcl`. It makes it more confusing to determine what is shared configuration and what is configuration for a unit.

It also complicates Terragrunt usage, as commands like `run-all` need to be run from a directory where all child directories will be `terragrunt.hcl` files corresponding to units that need to be run.

## Recommended Solution

To simplify Terragrunt usage and make it more clear what the root configuration is, it is now recommended that users rename the root `terragrunt.hcl` file to something else (e.g. `root.hcl`).

This will simplify Terragrunt usage, as you will no longer need to carefully avoid running Terragrunt commands in a way that might make it think the root `terragrunt.hcl` file is unit configuration, and it will make it more obvious to users what is and isn't a unit.

Note that in addition to renaming the root `terragrunt.hcl` file, you will also need to update any Terragrunt configurations that assume the root configuration will be named `terragrunt.hcl`. The most common example of this would be usage of `find_in_parent_folders` without any arguments. By default, this will look for a `terragrunt.hcl` file, so you will need to update this to look for the new root configuration file name.

e.g.

```hcl
# /some/path/terragrunt.hcl
include "root" {
  path = find_in_parent_folders()
}
```

To:

```hcl
# /some/path/terragrunt.hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}
```

## Additional Considerations

If you use [Scaffold](/docs/features/scaffold) and [Catalog](/docs/features/catalog), you may need to use additional flags to control how new units are generated. It was previously a safe assumption that most users would leverage a root `terragrunt.hcl` file, and thus, the default behavior was to generate a new unit that would look for a `terragrunt.hcl` file above it.

You can use the `--root-file-name` and `--no-include-root` flags of both `catalog` and `scaffold` to explicitly control how new units are generated, and what they will look for as the root configuration file (or if they should look for one at all).

e.g.

```bash
terragrunt catalog
```

To:

```bash
terragrunt catalog --root-file-name root.hcl
```

## Strict Control

To enforce this recommended pattern, you can also enable the [root-terragrunt-hcl](/docs/reference/strict-mode/#root-terragrunt-hcl) strict control to throw an error when Terragrunt detects that a root `terragrunt.hcl` file is being used.

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

For now, warnings will be emitted when this pattern is detected in order to encourage users to change to the new pattern, but this behavior will be an explicit error in a future version of Terragrunt.

Given how long this has been the standard pattern, we want to assure users that they will have a _very_ long time to migrate to this new pattern. For the most part, using the old pattern results in very little practical difference in Terragrunt behavior, assuming Terragrunt usage is already working fine.

As an explicit promise, Terragrunt will not remove support for the old pattern until at least Terragrunt 2.0, and even then, it will be a removal with a long warning period. You can take your time to migrate to the new pattern for older codebases, and are encouraged to share any feedback you have on this change so that we can make it as smooth a transition as possible for you.

## Frequently Asked Questions

### Could a different default value be used for `find_in_parent_folders` (e.g. `root.hcl`)?

Yes, it could, but this would be a different, immediate breaking change as users might have both `root.hcl` files and `terragrunt.hcl` files in their repositories, and this could result in Terragrunt finding the wrong configuration file.

It also doesn't address a significant part of the problem, which is that the following frequently confuses new users to Terragrunt:

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

It does not communicate _what_ Terragrunt will look to include in parent folders, and having a hidden extra fallback value is not a good pattern for Terragrunt to encourage.

Furthermore, `find_in_parent_folders` _already_ supports a fallback value in the second parameter, when used. Having two different ways to specify a fallback value would be confusing.

Lastly, the `root` include does not have any special meaning in Terragrunt, from a functional perspective, it's merely a convention. Terragrunt users do not have to supply a root include, and users can have as many includes as they like. By requiring that users specify the root include filename explicitly, Terragrunt is encouraging users to think about what the root configuration is, and what they want in it.

### Is it better for the root configuration to be named `root.hcl`?

Naming the root file `root.hcl` is the recommended pattern, but it is not a requirement.

Our documentation and examples are updated to reference this new pattern, and following this pattern will allow users to pattern match when writing their own configurations.

### Is there any name I _shouldn't_ use for the root configuration?

The only names that we would strongly encourage you don't adopt for root configuration is any name that begins with `terragrunt` (e.g. `terragrunt.hcl` or `terragrunt.stack.hcl`).

It is not formally a reserved name, but there are currently only two special filenames in Terragrunt:

1. `terragrunt.hcl` - The default configuration file name for a Terragrunt unit.
2. `terragrunt.stack.hcl` - The default configuration file name for a Terragrunt stack.

Using a name that begins with `terragrunt` could cause confusion for users, as they might expect that Terragrunt has special behavior for files with that name.
