---
layout: collection-browser-doc
title: RFC Template for Contributors
category: RFC
categories_url: rfc
excerpt: This is a template you can use for proposing new major features to Terragrunt.
tags: ["rfc", "contributing", "community"]
order: 501
nav_title: Documentation
nav_title_link: /docs/
---

# RFC Template

What follows is the legacy template for RFCs. If you would like to submit an RFC, please use the new template, which can be found [here](https://github.com/gruntwork-io/terragrunt/issues/new/choose).
Future RFCs can be found by searching for RFCs in [the GitHub Issues tab](https://github.com/gruntwork-io/terragrunt/issues?q=is%3Aopen+is%3Aissue+label%3Arfc).

---

This is a template you can use for proposing new major features to Terragrunt. When creating a new RFC, copy this
template and fill in each respective section.

**STATUS**: In proposal _(This should be updated when you open a PR for the implementation)_


## Background

This section should describe why you need this feature in Terragrunt. This should include a description of:

- The problem you are trying to solve.
- The reason why Terraform can't solve this problem, or data points that suggest there is a long enough time horizon for
  implementation that it makes sense to workaround it in Terragrunt.
- Use cases for the problem. Why is it important that we have this feature?


## Proposed solution

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

This section should include any links that are helpful for background reading such as:

- Relevant issues
- Links to PRs: at a minimum, the initial PR for the RFC, and the implementation PR.
- Links to Terragrunt releases, if the proposed solution has been implemented.
