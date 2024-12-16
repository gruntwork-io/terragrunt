---
layout: collection-browser-doc
title: Terminology
category: getting-started
excerpt: Quickly understand commonly use terms in Terragrunt.
tags: ["terminology", "glossary"]
order: 104
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

Units typically represent a minimal useful piece of infrastructure that should be independently managed.

e.g. A unit might represent a single VPC, a single database, or a single server.

While not a requirement, a general tendency experienced when working with Terragrunt is that units tend to decrease in size. This is because Terragrunt makes it easy to segment pieces of infrastructure into their own state, and to have them interact with each other through the use of [dependency blocks](/docs/reference/config-blocks-and-attributes#dependency). Smaller units are quicker to update, easier to reason about and safer to work with.

A common pattern used in the repository structure for Terragrunt projects is to have a single `terragrunt.hcl` file located at the root of the repository, and multiple subdirectories each containing their own `terragrunt.hcl` file. This is typically done to promote code-reuse, as it allows for any configuration common to all units to be defined in the root `terragrunt.hcl` file, and for unit-specific configuration to be defined in child directories. In this pattern, the root `terragrunt.hcl` file is not considered a unit, while all the child directories containing `terragrunt.hcl` files are.

Note that units don't technically need to call their configuration files `terragrunt.hcl` (that's configurable via the [--terragrunt-config](/docs/reference/cli-options/#terragrunt-config)), and users don't technically need to use a root `terragrunt.hcl` file or to name it that. This is the most common pattern followed by the community, however, and deviation from this pattern should be justified in the context of the project. It can help others with Terragrunt experience understand the project more easily if industry standard patterns are followed.

### Stack

A stack is a collection of units managed by Terragrunt. There is ([as of writing](https://github.com/gruntwork-io/terragrunt/issues/3313)) work underway to provide a top level artifact for interacting with stacks via a `terragrunt.stack.hcl` file, but as of now, stacks are generally defined by a directory with a tree of units. Units within a stack can be dependent on each other, and can be updated in a specific order to ensure that dependencies are resolved in the correct order.

Stacks typically represent a collection of units that need to be managed in concert.

e.g. A stack might represent a collection of units that together form a single application environment, a business unit, or a region.

The design of `terragrunt.stack.hcl` files is to ensure that they function entirely as a convenient shorthand for an equivalent directory structure of units. This is to ensure that users are able to easily transition between the two paradigms, and are able to decide for themselves which approach to structuring infrastructure is most appropriate for their use case.

### Module

A module is an [OpenTofu/Terraform construct](https://opentofu.org/docs/language/modules/) defined using a collection of OpenTofu/Terraform configurations ending in `.tf` (or `.tofu` in the case of OpenTofu) that represent a general pattern of infrastructure that can be instantiated multiple times.

Modules typically represent a generic pattern of infrastructure that can be instantiated multiple times, with different configurations exposed as variables.

e.g. A module might represent a generic pattern for a VPC, a database, or a server. Note that this differs from a unit, which represents a single instance of a provisioned VPC, database, or server.

Modules can be located either in the local filesystem, in a remote repository, or in any of [these supported locations](https://opentofu.org/docs/language/modules/sources/).

To integrate a module into a Terragrunt unit, reference the module using the `source` attribute of the [terraform block](/docs/reference/config-blocks-and-attributes/#terraform).

Terragrunt users typically spend a good deal of time authoring modules, as they are the primary way of defining the infrastructure patterns that Terragrunt is going to be orchestrating. Using tooling like [Terratest](https://github.com/gruntwork-io/terratest) can help to ensure that modules are well-tested and reliable.

A common pattern in Terragrunt usage is to only ever provision versioned, immutable modules. This is because Terragrunt is designed to be able to manage infrastructure over long periods of time, and it is important to be able to reproduce the state of infrastructure at any point in time.

### Resource

A resource is a low level building block of infrastructure that is defined in OpenTofu/Terraform configurations.

Resources are typically defined in modules, but don't have to be. Terragrunt can provision resources defined with `.tf` files that are not part of a module, located adjacent to the `terragrunt.hcl` file of a unit.

e.g. A resource might represent a single S3 bucket, or a single load balancer.

Resources generally correspond to the smallest piece of infrastructure that can be managed by OpenTofu/Terraform, and each resource has a specific address in state.

### State

Terragrunt stores the current state of infrastructure in one or more OpenTofu/Terraform [state files](https://opentofu.org/docs/language/state/).

State is an extremely important concept in the context of OpenTofu/Terraform, and it's helpful to read the relevant documentation there to understand what Terragrunt does to it.

Terragrunt has myriad capabilities that are designed to make working with state easier, including automatically provisioning state backend resources, managing unit interaction with external state, and segmenting state.

The most common way in which state is segmented in Terragrunt projects is to take advantage of filesystem directory structures. Most Terragrunt projects are configured to store state in remote backends like S3 with keys that correspond to the relative path to the unit directory within a project, relative to the root `terragrunt.hcl` file.

### Directed Acyclic Graph (DAG)

The way in which units are resolved within a stack is via a [Directed Acyclic Graph (DAG)](https://en.wikipedia.org/wiki/Directed_acyclic_graph#:~:text=A%20directed%20acyclic%20graph%20is,a%20path%20with%20zero%20edges).

This graph is also used to determine the order in which resources are resolved within a unit. Dependencies in a DAG determine the order in which resources are created, updated, or destroyed.

For creations and updates, resources are updated such that dependencies are always resolved before their dependents. For destructions, resources are destroyed such that dependents are always destroyed before their dependencies.

This is still true even when working with multiple units in a stack. Terragrunt will resolve the dependencies of all units in a stack (resolving the DAG within each unit first), and then apply the changes to all units in the stack in the correct order.

Note that DAGs are _Acyclic_, meaning that there are no loops in the graph. This is because loops would create circular dependencies, which would make it impossible to determine the correct order to resolve resources.

### Don't Repeat Yourself (DRY)

The [Don't Repeat Yourself (DRY)](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) principle is a software development principle that states that duplication in code should be avoided.

Early on, a lot of Terragrunt functionality was designed to make it easier to follow the DRY principle. This was because Terraform users at the time found that they were often repeating the same, or very similar code across multiple configurations. Examples of this included the limitation that remote state and provider configurations needed to be repeated in every root module, and that there were limitations in the dynamicity of these configurations.

Over time, Terragrunt has evolved to provide more features that make it easier to manage infrastructure at scale, and the focus has shifted to offering more tooling for _orchestrating_ infrastructure, rather than simply making it easier to avoid repeating yourself. Many of the features still serve to make it easier to follow the DRY principle, but this is no longer the primary focus of the tool.

Much of the marketing around Terragrunt still emphasizes the DRY principle, as it is a useful way to explain the value of Terragrunt to new users. However, you might miss the forest for the trees if you focus too much on the DRY principle when evaluating Terragrunt. Terragrunt is a powerful tool that can be used to manage infrastructure at scale, and it is worth evaluating it based on its capabilities to do so.

### Blast Radius

[Blast Radius](https://en.wikipedia.org/wiki/Blast_radius) is a term used in software development to describe the potential impact of a change, derived from the term used to describe the potential impact of an explosion.

In the context of infrastructure management, blast radius is used to describe the potential impact (negative or positive) of a change to infrastructure. The larger the blast radius, the more potential impact a change has.

Terragrunt was born out of a need to reduce the blast radii of infrastructure changes. By making it easier to segment state in infrastructure, and to manage dependencies between units, Terragrunt makes it easier to reason about the impact of changes to infrastructure, and to ensure that changes can be made safely.

When using Terragrunt, there is very frequently a mapping between your filesystem and the infrastructure you have provisioned with OpenTofu/Terraform. As such, when changing your current working directory in a Terragrunt project, you end up implicitly changing the blast radius of Terragrunt commands. The more units you have as children of your current working directory (the units in your stack), the more infrastructure you are likely to impact with a Terragrunt command.

As an adage, you can generally think of this property as: "Your current working directory is your blast radius".

### Run

A run is a single invocation of OpenTofu/Terraform by Terragrunt.

Runs are the primary way that Terragrunt does work. When you run `terragrunt plan` or `terragrunt apply`, Terragrunt will invoke OpenTofu/Terraform to drive the infrastructure update accordingly.

Note that runs abstract away a lot of the complexity that comes from working with OpenTofu/Terraform directly. Terragrunt might automatically perform some code generation, provision requisite resources, or add/modify to the underlying OpenTofu/Terraform configuration to ensure that day to day operations are as smooth as possible.

The way in which these complexities are abstracted is via Terragrunt configuration files (`terragrunt.hcl`), which can be used to define how Terragrunt should forward commands to OpenTofu/Terraform.

In the simplest case, a run in a unit with an empty `terragrunt.hcl` file will be equivalent to running OpenTofu/Terraform directly in the unit directory (with some small additional features like automatic initialization and logging adjustments).

### Execution

An execution is a single command run by Terragrunt, which does not necessarily have anything to do with OpenTofu/Terraform.

Ways in which Terragrunt can perform executions are limited to features like [hooks](/docs/features/hooks/), [run_cmd](/docs/reference/built-in-functions/#run_cmd), etc.

These utilities are part of what makes Terragrunt so powerful, as they allow users to move infrastructure management complexity out of modules.

### Run Queue

The Run Queue is the queue of all units that Terragrunt will do work on over one or more runs.

Certain commands like [run-all](/docs/reference/cli-options/#run-all) populate the Run Queue with all units in a stack, while other commands like `plan` or `apply` will only populate the Run Queue with the unit that the command was run in.

Certain flags like [--terragrunt-include-dir](/docs/reference/cli-options/#terragrunt-include-dir) can be used to adjust the Run Queue to include additional units. Conversely, there are flags like [--terragrunt-exclude-dir](/docs/reference/cli-options/#terragrunt-exclude-dir) that can be used to adjust the Run Queue to exclude units.

Terragrunt will always attempt to run until the Run Queue is empty.

### Runner Pool

The Runner Pool is the pool of available resources that Terragrunt can use to execute runs.

Units are dequeued from the Runner Pool into the Runner Pool depending on factors like [terragrunt-parallelism](/docs/reference/cli-options/#terragrunt-parallelism) and the DAG.

Units are only considered "running" when they are in the Runner Pool.

### Dependency

A dependency is a relationship between two units in a stack that results in data being passed from the dependency to the dependent unit.

Dependencies are defined in Terragrunt configuration files using the [dependency block](/docs/reference/config-blocks-and-attributes#dependency).

Dependencies are important for resolving the DAG, and the DAG is one of the most important properties to understand with Terragrunt. In an effort to avoid confusing users, Terragrunt maintainers attempt to overload the term "dependency" as little as possible. Other relationships may be described as "reading" or "including" to avoid any ambiguity as to what is relevant to the DAG.

### Include

The term "include" is used in two different contexts in Terragrunt.

1. **Include in configuration**: This is when one configuration file is included as partial configuration in another configuration file. This is done using the [include block](/docs/reference/config-blocks-and-attributes#include) in Terragrunt configuration files.
2. **Include in the Run Queue**: This is when a unit is included in the Run Queue. There are multiple ways for a unit to be included in the Run Queue.

### Exclude

The term "exclude" is only used in the context of excluding units from the Run Queue.

### Variable

A variable is a named dynamic value that is exposed by OpenTofu/Terraform configurations.

To avoid ambiguity, Terragrunt maintainers try to avoid using the term "variable" in Terragrunt documentation.

### Input

An input is a value configured in Terragrunt configurations to set the value of OpenTofu/Terraform variables.

Inputs are defined in Terragrunt configuration files using the [inputs attribute](/docs/reference/config-blocks-and-attributes#inputs). Under the hood, these inputs result in `TF_VAR_` prefixed environment variables being populated before initiating a run.

### Output

An output is a value that is returned by OpenTofu/Terraform after a run is completed.

By default, Terragrunt will interact with OpenTofu/Terraform in order to retrieve these outputs via [dependency blocks](/docs/reference/config-blocks-and-attributes#dependency).

Terragrunt does have the ability to mock outputs, which is useful when dependencies do not yet have outputs to be consumed (e.g. during the run of a unit with a dependency that has not been applied).

Terragrunt also has the ability to fetch outputs without interacting with OpenTofu/Terraform via [--terragrunt-fetch-dependency-output-from-state](/docs/reference/cli-options/#terragrunt-fetch-dependency-output-from-state) for dependencies where state is stored in AWS. This is an experimental feature, and more tooling is planned to make this easier to use.

### Feature

A [feature](/docs/reference/cli-options/#feature) is a configuration that can be dynamically controlled in Terragrunt configurations.

They operate very similarly to variables, but are designed to be used to dynamically adjust the behavior of Terragrunt configurations, rather than OpenTofu/Terraform configurations.

Features can be adjusted using feature flags, which are set in Terragrunt configurations using the [feature block](/docs/reference/config-blocks-and-attributes#feature) and the [feature flag](/docs/reference/config-blocks-and-attributes#feature-flag) attribute.

Like all good feature flags, you are encouraged to use them with good judgement and to avoid using them as a crutch to avoid making decisions about permanent adjustments to your infrastructure.

### IaC Engine

[IaC Engines](/docs/features/engine/) (typically abbreviated "Engines") are a way to extend the capabilities of Terragrunt by allowing users to control exactly how Terragrunt performs runs.

Engines allow Terragrunt users to author custom logic for how runs are to be executed in plugins, including defining exactly how OpenTofu/Terraform is to be invoked, where OpenTofu/Terraform is to be invoked, etc.

### Infrastructure Estate

An infrastructure estate is all the infrastructure that a person or organization manages. This can be as small as a single resource, or as large as a collection of repositories containing one or more stacks.

Generally speaking, the larger the infrastructure estate, the more important it is to have good tooling for managing it. Terragrunt is designed to be able to manage infrastructure estates of any size, and is used by organizations of all sizes to manage their infrastructure efficiently.

## CLI Redesign

Note that some of the language used in this page may be adjusted in the near future due to RFC [#3445](https://github.com/gruntwork-io/terragrunt/issues/3445).

To make terminology and overall UI/UX of using Terragrunt more consistent and easier to understand, the RFC proposes a number of changes to the CLI. This includes renaming some flags, reorganizing some commands, and adjusting some terminology.

As of this writing, the RFC is still in the proposal stage, so share your thoughts on the RFC if you have any opinions on the proposed changes.
