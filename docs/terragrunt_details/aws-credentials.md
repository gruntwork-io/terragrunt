---
title: AWS Credentials
layout: single
author_profile: true
sidebar:
  nav: "docs"
---

Terragrunt uses the official [AWS SDK for Go](https://aws.amazon.com/sdk-for-go/), which
means that it will automatically load credentials using the
[AWS standard approach](https://aws.amazon.com/blogs/security/a-new-and-standardized-way-to-manage-credentials-in-the-aws-sdks/). If you need help configuring your credentials, please refer to the [Terraform docs](https://www.terraform.io/docs/providers/aws/#authentication).
