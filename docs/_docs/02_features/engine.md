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
   source  = "/home/users/iac-engines/terragrunt-iac-engine-opentofu_v0.0.1"
   # Optionally set metadata for the plugin.
   meta = { 
     key_1 = ["value1", "value2"]
     key_2 = "1.6.0"
   }
}
```

Parameters:

* `source`: (Required) The source of the plugin. Currently, only local paths are supported.
* `meta`: (Optional) Block for setting plugin-specific metadata.

Due to the fact that this functionality is still experimental, and not recommended for general production usage, set the following environment variable to opt-in to this functionality:

```sh
export TG_EXPERIMENTAL_ENGINE=1
```

You can find the OpenTofu engine on Github [terragrunt-engine-opentofu](https://github.com/gruntwork-io/terragrunt-engine-opentofu).
