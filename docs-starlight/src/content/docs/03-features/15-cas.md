---
title: Content Addressable Store (CAS)
description: Learn how Terragrunt supports deduplication of content using a Content Addressable Store (CAS).
slug: docs/features/cas
sidebar:
  order: 15
---

Terragrunt supports a Content Addressable Store (CAS) to deduplicate content across multiple Terragrunt configurations. This feature is still experimental and not recommended for general production usage.

The CAS is used to speed up both catalog cloning and OpenTofu/Terraform source cloning by avoiding redundant downloads of Git repositories.

To use the CAS, you will need to enable the [cas](/docs/reference/experiments/#cas) experiment.

## Usage

When you enable the `cas` experiment, Terragrunt will automatically use the CAS when cloning any compatible source (Git repositories).

### Catalog Usage

```hcl
# root.hcl

catalog {
  urls = [
    "git@github.com:acme/modules.git"
  ]
}
```

### OpenTofu/Terraform Source Usage

```hcl
# terragrunt.hcl

terraform {
  source = "git@github.com:acme/infrastructure-modules.git//vpc?ref=v1.0.0"
}
```

When Terragrunt clones a repository while using the CAS, if the repository is not found in the CAS, Terragrunt will clone the repository from the original URL and store it in the CAS for future use.

When generating a repository from the CAS, Terragrunt will hard link entries from the CAS to the new repository. This allows Terragrunt to deduplicate content across multiple repositories.

In the event that hard linking fails due to some operating system / host incompatibility with hard links, Terragrunt will fall back to performing copies of the content from the CAS.

## Storage

The CAS is stored in the `~/.cache/terragrunt/cas` directory. This directory can be safely deleted at any time, as Terragrunt will automatically regenerate the CAS as needed.

Avoid partial deletions of the CAS directory without care, as that might result in partially cloned repositories and unexpected behavior.
