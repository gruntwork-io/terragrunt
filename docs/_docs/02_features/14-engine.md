---
layout: collection-browser-doc
title: IaC Engines
category: features
categories_url: features
excerpt: Learn how to dynamically control OpenTofu/Terraform runs using IaC engines.
tags: ["engine"]
order: 214
nav_title: Documentation
nav_title_link: /docs/
slug: engine
---

IaC engines allow you to customize and configure how IaC updates are orchestrated by Terragrunt. This feature is still experimental and not recommended for general production usage.

To try it out, all you need to do is include the following in your `terragrunt.hcl`:

```hcl
engine {
  source  = "github.com/gruntwork-io/terragrunt-engine-opentofu"
  version = "v0.0.7"
}
```

This example leverages the official OpenTofu engine, [publicly available on GitHub](https://github.com/gruntwork-io/terragrunt-engine-opentofu).

This engine currently leverages the locally available installation of the `tofu` binary, just like Terragrunt does by default without use of engine configurations. It provides a convenient example of how to build engines for Terragrunt.

In the future, this engine will expand in capability to include additional features and configurations.

Due to the fact that this functionality is still experimental, and not recommended for general production usage, set the following environment variable to opt-in to this functionality:

```sh
export TG_EXPERIMENTAL_ENGINE=1
```

## Use Cases

IaC Engines were introduced in order to offer advanced users of Terragrunt a level of customization over how exactly IaC updates are performed with a given set of Terragrunt configurations.

Without usage of IaC Engines, Terragrunt will determine how IaC updates are going to be performed by doing things like invoking the `tofu` or `terraform` binary directly. For most users, this is fine.

However, advanced users have more complex use cases that require more control over how those IaC updates are executed, given certain Terragrunt configurations.

e.g.

* Emitting custom logging or metrics whenever the `tofu` binary is executed.
* Running `tofu` in a remote environment, such as a separate Kubernetes pod from the one executing Terragrunt.
* Using different versions of `tofu` for different Terragrunt configurations in the same `run --all` execution.

## HTTPS Sources

Use an HTTP(S) URL to specify the path to the engine:

```hcl
engine {
  source  = "https://github.com/gruntwork-io/terragrunt-engine-opentofu/releases/download/v0.0.5/terragrunt-iac-engine-opentofu_rpc_v0.0.5_linux_amd64.zip"
}

```

## Local Sources

Specify a local absolute path as the source:

```hcl
engine {
   source  = "/home/users/iac-engines/terragrunt-iac-engine-opentofu_v0.0.1"
}
```

## Parameters

* `source`: (Required) The source of the plugin. Multiple engine approaches are supported, including GitHub repositories, HTTP(S) paths, and local absolute paths.
* `version`: The version of the engine to download from GitHub releases, if not specified, the latest release is always downloaded.
* `type`: (Optional) Currently, the only supported type is `rpc`.
* `meta`: (Optional) A block for setting engine-specific metadata. This can include various configuration settings required by the engine.

## Caching

Engines are cached locally by default to enhance performance and minimize repeated downloads.

The cached engines are stored in the following directory:

```sh
~/.cache/terragrunt/plugins/iac-engine/rpc/<version>
```

If you need to use a different path, set the environment variable `TG_ENGINE_CACHE_PATH` accordingly.

Downloaded engines are checked for integrity using the SHA256 checksum GPG key.
If the checksum does not match, the engine is not executed.
To disable this feature, set the environment variable:

```sh
export TG_ENGINE_SKIP_CHECK=0
```

To configure a custom log level for the engine, set the `TG_ENGINE_LOG_LEVEL` environment variable to one of: `debug`, `info`, `warn`, `error`.

```sh
export TG_ENGINE_LOG_LEVEL=debug
```

## Engine Metadata

The `meta` block is used to pass metadata to the engine. This metadata can be used to configure the engine or pass additional information to the engine.

The metadata block is a map of key-value pairs. Engines can read the information passed via the metadata map to configure themselves.

```hcl
engine {
   source  = "/home/users/iac-engines/my-custom-engine"
   # Optionally set metadata for the engine.
   meta = {
     key_1 = ["value1", "value2"]
     key_2 = "1.6.0"
   }
}
```

Configurations you might want to set with `meta` include:

* Connection configurations
* Tool versions
* Feature flags
* Other configurations that the engine might want to be variable in different `terragrunt.hcl` files
