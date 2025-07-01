---
layout: collection-browser-doc
title: Bare Include
category: migrate
categories_url: migrate
excerpt: Migration guide to rewrite configurations to avoid using bare includes
tags: ["migration", "community"]
order: 605
nav_title: Documentation
nav_title_link: /docs/
slug: bare-include
---

## Migrating from bare includes

The earliest form of include support in Terragrunt was a bare include.

e.g.

```hcl
include {
    path = find_in_parent_folders("root.hcl")
}
```

Once Terragrunt supported the ability to define multiple includes, and to expose the values in includes as variables, users could optionally use named includes instead of a bare include.

e.g.

```hcl
include "root" {
    path = find_in_parent_folders("root.hcl")
}
```

HCL parsing does not support the ability to parse HCL configuration and accept that a configuration block has zero or more attributes, so a workaround in Terragrunt internals was to parse the configuration, then rewrite it internally to avoid breaking backwards compatibility for bare includes.

e.g.

```hcl
include {
    path = find_in_parent_folders("root.hcl")
}
```

becomes:

```hcl
include "" {
    path = find_in_parent_folders("root.hcl")
}
```

Especially on large projects, this extra work is not worth the performance penalty, and Terragrunt has deprecated support for bare includes.

In a future version of Terragrunt, users will be required to use named includes for all includes.
