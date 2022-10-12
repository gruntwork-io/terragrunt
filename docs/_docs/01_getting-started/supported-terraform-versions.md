---
layout: collection-browser-doc
title: Terraform Version Compatibility Table
category: getting-started
excerpt: Learn which Terraform versions are compatible with which versions of Terragrunt.
tags: ["install"]
order: 102
nav_title: Documentation
nav_title_link: /docs/
---

## Supported Terraform Versions

The officially supported versions are:

| Terraform Version | Terragrunt Version                                                                                                                                    |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1.3.x             | >= [0.40.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.40.0)                                                                          |
| 1.2.x             | >= [0.38.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.38.0)                                                                          |
| 1.1.x             | >= [0.36.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.36.0)                                                                          |
| 1.0.x             | >= [0.31.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.31.0)                                                                          |
| 0.15.x            | >= [0.29.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.29.0)                                                                          |
| 0.14.x            | >= [0.27.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.27.0)                                                                          |
| 0.13.x            | >= [0.25.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.25.0)                                                                          |
| 0.12.x            | [0.19.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.19.0) - [0.24.4](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.24.4) |
| 0.11.x            | [0.14.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.14.0) - [0.18.7](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.18.7) |


However, note that these are the versions that are officially tested in the CI process. In practice, the version
compatibility is more relaxed than documented above. For example, we've found that Terraform 0.13 works with any version
above 0.19.0, and we've also found that terraform 0.11 works with any version above 0.19.18 as well.

If you wish to use Terragrunt against an untested Terraform version, you can use the
[terraform_version_constraint](https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes/#terraform_version_constraint)
(introduced in Terragrunt [v0.19.18](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.19.18)) attribute to
relax the version constraint.
