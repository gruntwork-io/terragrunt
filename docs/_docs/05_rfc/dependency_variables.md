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

This section should describe why you need this feature in Terragrunt. This should include a description of:

- The problem you are trying to solve.
- The reason why Terraform can't solve this problem, or data points that suggest there is a long enough time horizon for
  implementation that it makes sense to workaround it in Terragrunt.
- Use cases for the problem. Why is it important that we have this feature?


## Proposed solution

Introduces the ability to pass environment variables to terragrunt configurations which are referenced using the dependency block. There can be general environment variables being passed to every dependency but also environment variables (merged additively) per dependency. The existing get_env function including falling to a default to retreive a value and act accordingly.

This section should describe which solution you are ultimately picking for implementation. This should describe in
detail how this solution addresses the problem. Additionally, this section should include implementation details. Be
sure to include code samples so that it is clear how users are intended to use the solution you are proposing!

Note: This section can be left blank in the initial PR, if you are unsure what the best solution is. In this case,
include all the possible solutions you can think of as a part of the `Alternatives` section below. You can fill this
section out once you feel confident in a potential approach after discussion on the PR.

## Alternatives

This section should describe various options for resolving the problem stated in the `Background` section. Be sure to
include various alternatives here and not just the one you are ultimately proposing. This helps communicate what
tradeoffs you are making in picking your solution. Any reasonably complex problem that requires an RFC have multiple
solutions that appear to be valid, so it helps to be explicit about why certain solutions were not chosen.

## References

- Original issue: [#2674](https://github.com/gruntwork-io/terragrunt/issues/2674)
- RFC PR: [!2759](https://github.com/gruntwork-io/terragrunt/pull/2759)
- Draft PR: [!2759](https://github.com/gruntwork-io/terragrunt/pull/2759)
