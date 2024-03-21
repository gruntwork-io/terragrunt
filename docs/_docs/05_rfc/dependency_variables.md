---
layout: collection-browser-doc
title: Passing variables to dependencies
category: RFC
categories_url: rfc
excerpt: Allowing more flexibility in behaviour when the directory structure is not the full source of truth.
tags: ["rfc", "contributing", "community"]
order: 505
nav_title: Documentation
nav_title_link: /docs/
---

# RFC: Passing variables to dependencies

**STATUS**: In proposal

## Background

I'd like to have a function or variable enabling me to distinguish whether the current terragrunt execution was initiated by another terragrunt run. This is e.g. the scenario when you reference another directory containing a terragrunt.hcl in a dependency block. A way to handle those scenarios would be beneficial when e.g. another state would need be addressed when fetching outputs using the dependency block.

We distinguish states partly by using environment variables where we would have 60+ directories (each environment) for 30+ teams. This does work quite well except when a dependency block is used and a state file should be referenced which is not addressable using the environment variables. The address tho can be inferred by running a script using run_cmd to get the components of the associated state.

An example would be like that. Files can be seen [here](https://github.com/juljaeg/terragrunt/tree/feature/dependency-variables-2674/test/fixture-dependency-variables). The flow would basically be this:

```bash
cd test/fixture-dependency-variables/module-a

VARIANT=a terragrunt apply
terragrunt apply
// Should have terraform-a.tfstate and terraform-default.tfstate now.

cd ../module-b
terragrunt apply
// Is:     Hello World, from B: Hello World, from A: default
// Should: Hello World, from B: Hello World, from A: a
```

Especially in large use cases where directories are not suitable anymore as single source of truth for information this is very helpful.

## Proposed solution

Introduce the ability to pass environment variables to terragrunt configurations which are referenced using the dependency block. There can be general environment variables being passed to every dependency but also environment variables (merged additively) per dependency. The existing get_env function including falling back to a default can be used to retreive a value and act accordingly.

## Alternatives

### Directory structure

We evaluated mirroring the state address components in the directory structure, but yielded this will become unmaintainable with growing number of teams and environments. Also it leads to a lot of duplication. Also environments come and go which would mean deleting directories across 30+ teams.

### Dedicated indicator function

Have a function e.g. `is_silent_run_by_terragrunt` which enables me to detect this scenario. Optionally this may be extended to also contain the operation e.g. output to further distinguish. Of course scenarios like run-all make this a bit hard to define when this function will return what value as run-all also runs terragrunt in a subdirectory.

## References

- Original issue: [#2674](https://github.com/gruntwork-io/terragrunt/issues/2674)
- RFC PR: [!2760](https://github.com/gruntwork-io/terragrunt/pull/2760)
- Draft PR: [!2759](https://github.com/gruntwork-io/terragrunt/pull/2759)
