---
layout: collection-browser-doc
title: variable and output modifications
category: rfc
categories_url: rfc
excerpt: variable and output modifications
tags: ["rfc", "contributing", "community"]
order: 505
nav_title: Documentation
nav_title_link: /docs/
---

# variable and output modifications

**STATUS**: In proposal


## Background

Terragrunt over the years has evolved to adapt to deploying shared modules from any source as root modules by injecting
various blocks and terraform code to support the deployment. In Terraform, modules can be loosely categorized into two types:

* **Root Module**: A Terraform module that is designed for running `terraform init` and the other workflow commands
  (`apply`, `plan`, etc). This is the entrypoint module for deploying your infrastructure. Root modules are identified
  by the presence of key blocks that setup configuration about how Terraform behaves, like `backend` blocks (for
  configuring state) and `provider` blocks (for configuring how Terraform interacts with the cloud APIs).
* **Shared Module**: A Terraform module that is designed to be included in other Terraform modules through `module`
  blocks. These modules are missing many of the key blocks that are required for running the workflow commands of
  terraform.

Note that Terragrunt is not designed to deploy any **Shared Module**. That is, modules that are necessary to be composed
with other modules should not really be deployed with Terragrunt. Terragrunt further distinguishes shared modules
between **service modules** and **modules**:

* **Shared Service Module**: A Terraform module that is designed to be standalone and applied directly. These modules
  are not root modules in that they are still missing the key blocks like `backend` and `provider`, but aside from that
  do not need any additional configuration or composition to deploy. For example, the
  [terraform-aws-modules/vpc](https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest) module can be
  deployed by itself without composing with other modules or resources.
* **Shared Module**: A Terraform module that is designed to be composed with other modules. That is, these modules must
  be embedded in another Terraform module and combined with other resources or modules. For example, the
  [consul-security-group-rules
  module](https://registry.terraform.io/modules/hashicorp/consul/aws/latest/submodules/consul-security-group-rules)

At its core, Terragrunt is designed to add the necessary ingredients to convert **Shared Service Modules** into **Root
Modules** so that they can be deployed with `terraform apply`. Features like `generate` and `remote_state` support this
transition by injecting the necessary blocks that support directly invoking `terraform`.

Note that the distinction between **Shared Service Modules** and **Shared Modules** is subtle, and oftentimes is not a
clear cut technical difference. However, there are technical limitations in the current Terragrunt implementation that
prevents deploying certain shared modules:

- Every complex input must have a `type` associated with it. Otherwise, Terraform will interpret the input that
  Terragrunt passes through as `string`. This includes `list` and `map`.
- Derived sensitive outputs must be marked as `sensitive`. Refer to the [terraform tutorial on sensitive
  variables](https://learn.hashicorp.com/tutorials/terraform/sensitive-variables#reference-sensitive-variables) for more
  information on this requirement.

Terraform only enforces these restrictions on the **Root Module**. This means that there are some shared modules on the
registry that Terragrunt can not deploy. Note that the lack of these parameters may not by itself indicate that they are
**Shared Modules**. That is, these modules may be **Shared Service Modules** by design, but because they are
only designed for use with Terraform, they may not set the `type` or `sensitive` flags on the `variable` and `output`
blocks, preventing usage as a transformed root module unless those inputs and outputs are configured.


## Proposed solution

To handle this, this RFC proposes a new block: `transform`. Here is an example:

Consider a module that has the following:

```hcl
variable "my_password" {
  sensitive = true
}

variable "my_list" {}

# NOTE: this must be marked as sensitive since it is derived from a sensitive variable
output "my_password_hashed" {
  value = base64sha256(var.my_password)
}

output "length_my_list" {
  value = length(var.my_list)
}
```

Using Terragrunt with this module will run into the following issue:

- Because the output `my_password_hashed` is not marked as sensitive, terraform will error out.
- `my_list` is missing the type definition, so the input from `terragrunt` will be interpretted as a string. This means
  that the output `length_my_list` will be the string length, and not the list length.

We will need to transform these variables and outputs. We will introduce the `transform` block to handle this. The
following `terragrunt.hcl` configuration indicates the necessary transformations to `variable` and `output` to support
deployment:

```hcl
transform {
  # Sub blocks are 'variable' or 'output', to indicate what terraform block needs to be transformed. Next, the label
  # should match with the corresponding variable label defined in the terraform module.
  # Each subblock matches the underlying terraform block, and under the hood, the attributes and blocks are merged into
  # the terraform module.
  # For example, in this first variable subblock, the 'type' attribute is merged into the upstream terraform module
  # before terragrunt invokes terraform.
  variable "my_list" {
    type = list(string)
  }
  output "my_password_hashed" {
    sensitive = true
  }
}
```

As indicated in the comment, Terragrunt will merge these `variable` and `output` configurations into the module code
prior to invoking Terraform. That is, the following will happen when `terragrunt apply` is invoked:

- Terragrunt parses the configurations. The transform operations are interpretted and recorded internally at this point.
- Terragrunt clones the module source into the working directory (`.terragrunt-cache`).
- `generate` blocks are processed and copied into the working directory.
- `transform` blocks are processed. In this stage, Terragrunt will scan the `variable` and `output` blocks in the
  underlying module cloned in the working directory, and **directly modify the local source** with the updates. In this
  case, the `my_list` variable block definition will have the `type = list(string)` attribute set, and the
  `my_password_hashed` output will have `sensitive = true` attribute set.
- Terragrunt invokes `terraform apply` on the modified module.

In this way, Terragrunt can convert the underlying shared module into a root module that can be deployed directly.
Under the hood, this transformation is implemented in the same way that the `aws-provider-patch` command works.


## Alternatives

### Null option: Wrapper modules

Currently, users are expected to work around this by creating a wrapper module that acts as the root module. That is,
the user implements a new Terraform module that wraps the underlying module that does not have variable types or
sensitive outputs, and implements those definitions. Note that this requires redefining and plumbing all the variables
and outputs that the user intends to use from the underlying module, which can be cumbersome to maintain in the long
run, assuming the user only needs that single module for deployment.


## References

- https://github.com/gruntwork-io/terragrunt/issues/1774
- https://github.com/gruntwork-io/terragrunt/issues/1808
