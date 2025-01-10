---
layout: collection-browser-doc
title: Terraform and OpenTofu Version Compatibility Table
category: reference
categories_url: reference
excerpt: Learn which Terraform and OpenTofu versions are compatible with which versions of Terragrunt.
tags: [ "install" ]
order: 406
nav_title: Documentation
nav_title_link: /docs/
---

## Supported OpenTofu Versions

The officially supported versions are:

| OpenTofu Version | Terragrunt Version                                                           |
|------------------|------------------------------------------------------------------------------|
| 1.9.x            | >= [0.72.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.72.0) |
| 1.8.x            | >= [0.66.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.66.0) |
| 1.7.x            | >= [0.58.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.58.0) |
| 1.6.x            | >= [0.52.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.52.0) |

## Supported Terraform Versions

The officially supported versions are:

| Terraform Version | Terragrunt Version                                                                                                                                    |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1.9.x             | >= [0.60.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.60.0)                                                                          |
| 1.8.x             | >= [0.57.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.57.0)                                                                          |
| 1.7.x             | >= [0.56.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.56.0)                                                                          |
| 1.6.x             | >= [0.53.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.53.0)                                                                          |
| 1.5.x             | >= [0.48.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.48.0)                                                                          |
| 1.4.x             | >= [0.45.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.45.0)                                                                          |
| 1.3.x             | >= [0.40.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.40.0)                                                                          |
| 1.2.x             | >= [0.38.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.38.0)                                                                          |
| 1.1.x             | >= [0.36.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.36.0)                                                                          |
| 1.0.x             | >= [0.31.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.31.0)                                                                          |
| 0.15.x            | >= [0.29.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.29.0)                                                                          |
| 0.14.x            | >= [0.27.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.27.0)                                                                          |
| 0.13.x            | >= [0.25.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.25.0)                                                                          |
| 0.12.x            | [0.19.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.19.0) - [0.24.4](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.24.4) |
| 0.11.x            | [0.14.0](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.14.0) - [0.18.7](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.18.7) |

**Note 1:** Terragrunt lists support for BSL versions of Terraform (>= 1.6.x) and core IaC functionality will work as expected.
However, support for BSL Terraform-specific features is not guaranteed even if that version is in this table.

**Note 2:** This table lists versions that are officially tested in the CI process. In practice, the version
compatibility is more relaxed than documented above. For example, we've found that Terraform 0.13 works with any version
above 0.19.0, and we've also found that terraform 0.11 works with any version above 0.19.18 as well.

If you wish to use Terragrunt against an untested Terraform version, you can use the
[terraform_version_constraint](https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes/#terraform_version_constraint)
(introduced in Terragrunt [v0.19.18](https://github.com/gruntwork-io/terragrunt/releases/tag/v0.19.18)) attribute to
relax the version constraint.
