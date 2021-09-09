---
layout: collection-browser-doc
title: Auto-retry
category: features
categories_url: features
excerpt: Auto-Retry is a feature of terragrunt that will automatically address situations where a terraform command needs to be re-run.
tags: ["CLI"]
order: 250
nav_title: Documentation
nav_title_link: /docs/
---

## Auto-Retry

*Auto-Retry* is a feature of `terragrunt` that will automatically address situations where a `terraform` command needs to be re-run.

Terraform can fail with transient errors which can be addressed by simply retrying the command again. In the event `terragrunt` finds one of these errors, the command will be re-run again automatically.

**Example**

```
$ terragrunt apply
...
Initializing provider plugins...
- Checking for available provider plugins on https://releases.hashicorp.com...
Error installing provider "template": error fetching checksums: Get https://releases.hashicorp.com/terraform-provider-template/1.0.0/terraform-provider-template_1.0.0_SHA256SUMS: net/http: TLS handshake timeout.
```

Terragrunt sees this error, and knows it is a transient error that can be addressed by re-running the `apply` command.

Terragrunt has a small list of default known errors built-in. You can override these defaults with your own custom retryable errors in your `terragrunt.hcl` configuration:
```hcl
retryable_errors = [
  "a regex to match the error",
  "another regex"
]
```

E.g:
```hcl
retryable_errors = [
  "(?s).*Error installing provider.*tcp.*connection reset by peer.*",
  "(?s).*ssh_exchange_identification.*Connection closed by remote host.*"
]
```

By default, `auto-retry` tries a maximum of three times to re-run a command, pausing for five seconds between each retry, at which point it will deem the error as not transient, and accept the `terraform` failure.
However, you can override these defaults. For example, the following retries up to five times, with 60 seconds in between each retry:

```hcl
retry_max_attempts = 5
retry_sleep_interval_sec = 60
```

To disable `auto-retry`, use the `--terragrunt-no-auto-retry` command line option or set the `TERRAGRUNT_AUTO_RETRY` environment variable to `false`.
