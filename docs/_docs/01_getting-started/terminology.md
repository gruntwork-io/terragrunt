---
layout: collection-browser-doc
title: Terminology
category: getting-started
excerpt: Quickly understand commonly use terms in Terragrunt.
tags: ["terminology", "glossary"]
order: 102
nav_title: Documentation
nav_title_link: /docs/
---

## Preamble

Infrastructure as Code (IaC) tooling necessarily requires a lot of terminology to describe various concepts and features due to the breadth of the domain.

Whenever possible, Terragrunt terminology attempts to align with wider industry standards, but there are always exceptions. There are going to be times when certain terms are used in different tools, but have special meaning in Terragrunt, and there are times when the same term might have different meaning in different contexts.

This document aims to provide a quick reference for the most important and commonly used terms in Terragrunt and generally in Gruntwork products. Whenever terminology used in Terragrunt deviates from this document, it should either be explained or adjusted to align with this document.


## Terms

### Terragrunt

Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in [OpenTofu](https://opentofu.org/)/[Terraform](https://www.terraform.io/) to scale.

It differs from many other IaC tools in that it is designed to be an orchestrator for OpenTofu/Terraform execution, rather than primarily provisioning infrastructure itself. Terragrunt users write OpenTofu/Terraform code to define high-level patterns of infrastructure that they want to create, then use Terragrunt to dynamically apply those generic patterns in particular ways.

Because of this separation of concerns, most of what Terragrunt does is designed to extend the capabilities of OpenTofu/Terraform, rather than replace them. Most of the [features](/docs#features) of Terragrunt are designed to make it easier to manage large infrastructure estates, or to provide additional capabilities that are inconvenient or impossible to achieve with OpenTofu/Terraform alone.

### OpenTofu

[OpenTofu](https://opentofu.org/) is an open-source Infrastructure as Code tool spawned as a fork of [Terraform](https://www.terraform.io/) after the license change from the [Mozilla Public License (MPL)](https://en.wikipedia.org/wiki/Mozilla_Public_License) to the [Business Source License (BSL)](https://en.wikipedia.org/wiki/Business_Source_License).

OpenTofu was created as a drop-in replacement for Terraform (as it was forked from the same MPL source code), and is designed to be fully compatible with Terraform configurations and modules.

You may notice that Terragrunt documentation uses the phrase "OpenTofu/Terraform" to refer to the IaC tooling that Terragrunt orchestrates. This is because Terragrunt is generally agnostic to the specific IaC tooling that is being used to drive infrastructure updates. When relevant, Terragrunt documentation endeavors to explicitly indicate that functionality is specific to one tool or the other. From the perspective of Terragrunt, the two are usually interchangeable, though Terragrunt will default to using OpenTofu if both are available.

Note that some documentation refers to Terraform alone in some instances as a consequence of the historical context in which Terragrunt was created, as it predates the creation of OpenTofu. Conversely, some documentation may refer to OpenTofu alone simply because of the fact that OpenTofu is the default IaC tool that Terragrunt uses.

### Unit

A unit is a single instance of infrastructure managed by Terragrunt. It has its own state, and can be detected by the presence of a `terragrunt.hcl` file in a directory.

A common pattern used in the repository structure for Terragrunt projects is to have a single `terragrunt.hcl` file located at the root of the repository, and multiple subdirectories each containing their own `terragrunt.hcl` file. This is typically done to promote code-reuse, as it allows for any configuration common to all units to be defined in the root `terragrunt.hcl` file, and for unit-specific configuration to be defined in child directories. In this pattern, the root `terragrunt.hcl` file is not considered a unit, while all the child directories containing `terragrunt.hcl` files are.

While not a requirement, a general tendency experienced when working with Terragrunt is that units tend to decrease in size. This is because Terragrunt makes it easy to segment pieces of infrastructure into their own state, and to have them interact with each other through the use of [dependency blocks](/docs/reference/config-blocks-and-attributes#dependency). Smaller units are quicker to update, easier to reason about and safer to work with.

Units typically represent a minimal useful piece of infrastructure that should be independently managed.

e.g. A unit might represent a single VPC, a single database, or a single server.

### Stack

A stack is a collection of units managed by Terragrunt. There is ([as of writing](https://github.com/gruntwork-io/terragrunt/issues/3313) work underway to provide a top level artifact for interacting with stacks via a `terragrunt.stack.hcl` file, but as of now, stacks are generally defined by a directory with a tree of units. Units within a stack can be dependent on each other, and can be updated in a specific order to ensure that dependencies are resolved in the correct order.

The design of `terragrunt.stack.hcl` files is to ensure that they function entirely as a convenient shorthand for an equivalent directory structure of units. This is to ensure that users are able to easily transition between the two paradigms, and are able to decide for themselves which approach to structuring infrastructure is most appropriate for their use case.

Stacks typically represent a collection of units that need to be managed in concert.

e.g. A stack might represent a collection of units that together form a single application environment, a business unit, or a region.

### Module

A module is a collection of OpenTofu/Terraform configurations defined with files ending in `.tf` (or `.tofu` in the case of OpenTofu) that represent a general pattern of infrastructure that can be instantiated multiple times.

Modules can be located either in the local filesystem, in a remote repository, or in any of [these supported locations](https://opentofu.org/docs/language/modules/sources/).

Terragrunt users typically spend a good deal of time authoring modules, as they are the primary way of defining the infrastructure patterns that Terragrunt is going to be orchestrating. Using tooling like [Terratest](https://github.com/gruntwork-io/terratest) can help to ensure that modules are well-tested and reliable.

A common pattern in Terragrunt usage is to only ever provision versioned, immutable modules. This is because Terragrunt is designed to be able to manage infrastructure over long periods of time, and it is important to be able to reproduce the state of infrastructure at any point in time.

Modules typically represent a generic pattern of infrastructure that can be instantiated multiple times, with different configurations exposed as variables.

e.g. A module might represent a generic pattern for a VPC, a database, or a server. Note that this differs from a unit, which represents a single instance of a provisioned VPC, database, or server.

### Resource

A resource is a low level building block of infrastructure that is defined in OpenTofu/Terraform configurations.

Resources generally correspond to the smallest piece of infrastructure that can be managed by OpenTofu/Terraform, and each resource has a specific address in state.

Resources are typically defined in modules, but don't have to be. Terragrunt can provision resources defined with `.tf` files that are not part of a module, located adjacent to the `terragrunt.hcl` file of a unit.

e.g. A resource might represent a single S3 bucket, or a single load balancer.

### Directed Acyclic Graph (DAG)

The way in which units are resolved within a stack is via a [Directed Acyclic Graph (DAG)](https://en.wikipedia.org/wiki/Directed_acyclic_graph#:~:text=A%20directed%20acyclic%20graph%20is,a%20path%20with%20zero%20edges).

This graph is also used to determine the order in which resources are resolved within a unit. Dependencies in a DAG determine the order in which resources are created, updated, or destroyed.

For creations and updates, resources are updated such that dependencies are always resolved before their dependents. For destructions, resources are destroyed such that dependents are always destroyed before their dependencies.

This is still true even when working with multiple units in a stack. Terragrunt will resolve the dependencies of all units in a stack (resolving the DAG within each unit first), and then apply the changes to all units in the stack in the correct order.

Note that DAGs are _Acyclic_, meaning that there are no loops in the graph. This is because loops would create circular dependencies, which would make it impossible to determine the correct order to resolve resources.

### Run

A run is a single invocation of OpenTofu/Terraform by Terragrunt.

### Infrastructure Estate

An infrastructure estate is all the infrastructure that a person or organization manages. This can be as small as a single resource, or as large as a collection of repositories containing one or more stacks.

Generally speaking, the larger the infrastructure estate, the more important it is to have good tooling for managing it. Terragrunt is designed to be able to manage infrastructure estates of any size, and is used by organizations of all sizes to manage their infrastructure efficiently.

