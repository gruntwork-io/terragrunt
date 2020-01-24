---
layout: collection-browser-doc
title: Auto-retry
category: Features
categories_url: features
excerpt: Auto-Retry is a feature of terragrunt that will automatically address situations where a terraform command needs to be re-run.
tags: ["CLI"]
order: 210
nav_title: Documentation
nav_title_link: /docs/
---

## Auto-Retry

*Auto-Retry* is a feature of `terragrunt` that will automatically address situations where a `terraform` command needs to be re-run.

Terraform can fail with transient errors which can be addressed by simply retrying the command again. In the event `terragrunt` finds one of these errors, the command will be re-run again automatically.

**Example**

    $ terragrunt apply
    ...
    Initializing provider plugins...
    - Checking for available provider plugins on https://releases.hashicorp.com...
    Error installing provider "template": error fetching checksums: Get https://releases.hashicorp.com/terraform-provider-template/1.0.0/terraform-provider-template_1.0.0_SHA256SUMS: net/http: TLS handshake timeout.

Terragrunt sees this error, and knows it is a transient error that can addressed by re-running the `apply` command.

`auto-retry` will try a maximum of three times to re-run the command, at which point it will deem the error as not transient, and accept the terraform failure. Retries will occur when the error is encountered, pausing for 5 seconds between retries.

Known errors that `auto-retry` will rerun, are maintained in the `TerragruntOptions.RetryableErrors` array. Future upgrades to `terragrunt` may include the ability to configure `auto-retry` by specifying additional error strings and configuring max retries and retry intervals the `terragrunt` config (PRs welcome\!).

To disable `auto-retry`, use the `--terragrunt-no-auto-retry` command line option or set the `TERRAGRUNT_AUTO_RETRY` environment variable to `false`.
