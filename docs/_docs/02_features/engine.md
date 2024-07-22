---
layout: collection-browser-doc
title: Terragrunt IAC Engine Plugin System
category: features
categories_url: features
excerpt: Terragrunt IaC engine
tags: ["engine"]
order: 313
nav_title: Documentation
nav_title_link: /docs/
---

## Terragrunt IAC Engine Plugin System

A new engine configuration block has been released allowing you to customize and configure how your IAC updates orchestrated by Terragrunt!

To try it out, all you need to do is include the following in your `terragrunt.hcl`:

```hcl
engine {
  source  = "github.com/gruntwork-io/terragrunt-engine-opentofu"
  version = "v0.0.2"
  type    = "rpc"
  # Optionally set metadata for the plugin.
  meta = {
    key_1 = ["value1", "value2"]
    key_2 = "1.6.0"
  }
}
```
Use an HTTP(S) URL to specify the path to the plugin:
```hcl
engine {
  source  = "https://github.com/gruntwork-io/terragrunt-engine-opentofu/releases/download/v0.0.2/terragrunt-iac-engine-opentofu_rpc_v0.0.2_linux_amd64.zip"
  version = "v0.0.2"
  type    = "rpc"
  # Optionally set metadata for the plugin.
  meta = {
    key_1 = ["value1", "value2"]
    key_2 = "1.6.0"
  }
}
```
Specify a local absolute path as the source:
```hcl
engine {
   source  = "/home/users/iac-engines/terragrunt-iac-engine-opentofu_v0.0.1"
   # Optionally set metadata for the plugin.
   meta = { 
     key_1 = ["value1", "value2"]
     key_2 = "1.6.0"
   }
}
```

Parameters:
* `source`: (Required) The source of the plugin. Multiple engine approaches are supported, including GitHub repositories, HTTP(S) paths, and local absolute paths.
* `version`: (Required for GitHub) The version of the plugin to download from GitHub releases.
* `type`: (Required) Currently, only `rpc` type is supported.
* `meta`: (Optional) A block for setting plugin-specific metadata. This can include various configuration settings required by the plugin.

Engines are cached locally to improve performance and reduce the need for repeated downloads. 
The cached engines are stored in the following directory:
```sh
~/.cache/terragrunt/plugins/iac-engine/rpc/<version>
```

Due to the fact that this functionality is still experimental, and not recommended for general production usage, set the following environment variable to opt-in to this functionality:
```sh
export TG_EXPERIMENTAL_ENGINE=1
```

You can find the OpenTofu Engine on Github [terragrunt-engine-opentofu](https://github.com/gruntwork-io/terragrunt-engine-opentofu).
