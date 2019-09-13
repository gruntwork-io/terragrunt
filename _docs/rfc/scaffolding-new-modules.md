# RFC: Scaffolding new live configurations using code generation

**STATUS**: In proposal


## Background

Doing anything that requires new Terragrunt configurations is currently painful. To highlight this, let's walk through
what you have to currently do for each of the following scenarios. For the purposes of this example, we will assume the
following infrastructure live folder structure:

```
.
├── stage
│   ├── terragrunt.hcl
│   └── us-east-1
│       └── stage
│           ├── vpc
│           │   └── terragrunt.hcl
│           ├── mysql
│           │   └── terragrunt.hcl
│           └── webserver-cluster
│               └── terragrunt.hcl
└── prod
    ├── terragrunt.hcl
    └── us-east-1
        └── prod
            ├── vpc
            │   └── terragrunt.hcl
            ├── mysql
            │   └── terragrunt.hcl
            └── webserver-cluster
                └── terragrunt.hcl
```

#### Adding a new component

Suppose we now want to add a new component to our stack, such as `redis`. We will assume that we have already defined
our infrastructure module for provisioning Redis, and we want to now add it to our stack by creating a new live config.

First, we need to decide where it should go in the folder structure. This is more or less clear in our example, where
our folder structure for our stack is flat. We will start in `stage` so we create the folder
`stage/us-east-1/stage/redis` as our home for the live config.

Next we need to start creating the `terragrunt.hcl` file. We start by specifying the boilerplate config we always need -
the `include` and `terraform` blocks:

```
terraform {
  source = "git::git@github.com:yourco/modules.git//redis?ref=v0.0.1"
}

include {
  path = find_in_parent_folders()
}
```

After this, we need to include the inputs. Here, we need to open up the `variables.tf` file in the `redis` module. Once
the `variables.tf` file is open, we need to start looking at which inputs we need to set. We first start with the
required ones. For this example, we will assume that our `redis` module needs a VPC and subnet. Since we don't want to
hard code these, we will add dependency blocks to look it up from our VPC module:

```
terraform {
  source = "git::git@github.com:yourco/modules.git//redis?ref=v0.0.1"
}

include {
  path = find_in_parent_folders()
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
  subent_ids = dependency.vpc.outputs.private_subnet_ids
}
```

Then we need to look through each optional input variable and decide which ones we need to set. Here we decide that we
need to set the `instance_type`:

```
terraform {
  source = "git::git@github.com:yourco/modules.git//redis?ref=v0.0.1"
}

include {
  path = find_in_parent_folders()
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  # Required Inputs
  vpc_id = dependency.vpc.outputs.id
  subent_ids = dependency.vpc.outputs.private_subnet_ids

  # Optional Inputs
  instance_type = "cache.t2.micro"
}
```

Since we want to make sure we don't lose context of required versus optional variables, we also add a comment indicating
as such.

Now that we have our `terragrunt.hcl`, we are ready to finally run `terragrunt apply`. Assuming it worked, the next step
is to promote it to our `prod` environment. To promote, we can copy paste what we have in `stage`. Since we are using
`dependency` blocks, the location of the module ensures we pull in the right VPC, so the only input we might need to
change for prod is `instance_type`, if we wanted a larger size for example.

At this point, we have successfully added the `redis` module to our infrastructure in both the `stage` and `prod`
environments. Given this procedure, here are the friction points:

- You have to copy paste the common boilerplate. If you remember this off the top of your head, you can type it out but
  99% of the time, you have to look this up. For a new person, this will not be immediately obvious. For example, how do
  you know that these two blocks are the boilerplate just by looking at the `mysql` live config?
- The variables exist in a different repository, and you need to see the reference to know which ones to set. Since
  there is no editor support, you can't avoid this without risking typos.
- You lose the context of the description. When you come back to update the vars, you will most likely need to reference
  the `variables.tf` file each time to know what each of the variables mean! Most people will not go through the trouble
  of copy pasting the description into the `terragrunt.hcl` file.

#### Adding a new environment, region, or account

Suppose that we want a new environment in the stage account that is for QA purposes. This is a replica of the stage
environment that lives in a different VPC.

To do this, we will copy paste the `stage/us-east-1/stage` folder to create `stage/us-east-1/qa`. Then we need to go
through and update all the variable references to reflect what we want in the QA environment, such as the name of
resources and instance sizes.

Adding a new region or account is similar. For a new region, instead of copying `stage/us-east-1/stage` you copy
`stage/us-east-1`. For a new account, you copy `stage`.

This procedure is similar to the promote step outlined in [Adding a new component](#adding-a-new-component). I won't go
through the details here since the procedure is relatively straightforward, but there are still some friction points:

- Like in [Adding a new component](#adding-a-new-component), it won't be obvious what each of the variables actually
  mean since the description is most likely not included. You will either rely on gut interpretation, your memory, or
  deduction by name to decide which ones need to be updated for a new environment.
- It isn't immediately obvious which variables need to actually change. In fact, you might have to update one of the
  defaults for your new environment! In this scenario, you will have to have a good understanding of the stack and the
  underlying `variables.tf` for each module to know this, or have the discipline to check each `variables.tf`. Both are
  unreasonable expectations.

---

Each of these friction points are currently resolved by operator discipline. As an operator of Terragrunt, you need to
make sure that each of your configs are sufficiently documented so that you know which variables need to be changed when
you want to replicate them. Alternatively, you need to know where to look for that information and have the discipline
to check each time. Both of these are unreasonable to expect for sufficiently large infrastructure deployments with many
possible modules and dependencies.

The goal of this RFC is to propose an approach to handling these basic tasks that minimizes the room for operator
errors.


## Proposed solution

We will introduce a new command `terragrunt new`, which provides a code generation interface for solving each of the
scenarios described above. The goal of `terragrunt new` is to provide the ability to templatize your configurations with
annotations in the code that reminds the user what pieces need to change for each scenario.

It is important that the templates are customizable. Not all users have the same requirements, so allowing users to
customize the templates to their needs is important. The goal is to allow users to define their own standards as
templates that then is used by their engineers to generate new configurations that conform to their standards, which may
be different from Gruntwork standards. I envision Gruntwork providing a set of base templates that can be used by
default, which can be overridden by either forking our `infrastructure-templates` folder or by building new ones from
scratch.

`terragrunt new` should support the following scenarios:

- `terragrunt new component`, which will add a new component to an existing environment. The inputs for this template
  should be:
    - Environment: which live environment folder should the component be added to?
    - Component module: which module should be used for the component?

- `terragrunt new environment`, which will add a new environment to an existing region. The inputs for this template
  should be:
    - Location: which live region/account folder should the environment be added to?
    - Stack: which stack should be used? This assumes you have different kinds of stacks in your environment, where a
      stack is a set of components.

- `terragrunt new region`, which will add a new region to an existing account. The inputs for this template should be:
    - Account: which live account folder should the region be added to? (NOTE: `account` may not be the right
      terminology here, as it does not mesh with GCP where you have Projects).

- `terragrunt new account`, which will add a new account. The inputs for this template should be:
    - Remote state info: the minimum information necessary to setup the remote state config.

- `terragrunt new wrapper-module`, which will add a new wrapper module (what you have in `infrastructure-modules`). Note
  that as such, this will be rendered into `infrastructure-modules`, as opposed to `infrastructure-live` like the other
  sub commands. The inputs for this template should be:
    - Module name: what to call this module.
    - Library Module source: what the underlying library module you are basing the wrapper on.

- `terragrunt new unicorn` (__PLACEHOLDER: I don't know what to call this__. Ideas: `other`, `arbitrary`, `config`),
  which will support adding any arbitrary template. The inputs depend on the selected template, but at the very least
  includes:
    - Template source: where does the template to use live?

### Setup

`terragrunt new` depends on a repository of templates. These templates can practically live anywhere, but for the
purposes of versioning, it should live outside of `infrastructure-live` and `infrastructure-modules`. For the purposes
of this example, we will assume all the templates live in a repository called `infrastructure-templates`.

To support each of the scenario above, `terragrunt` will need to be configured to know which subtemplates should be used
for each of the scenarios, in addition to knowing where the `infrastructure-live` and `infrastructure-modules`
repositories live. This should be configured in a new configuration file for Terragrunt, which will be autogenerated
from an interactive setup process the first time you run `terragrunt new`.

The following configuration options will be required:

- `infrastructure_live_repo`: Git URL of the `infrastructure-live` repository.
- `infrastructure_modules_repo`: Git URL of the `infrastructure-modules` repository.
- `infrastructure_templates_repo`: Git URL of the `infrastructure-templates` repository. If you follow the convention,
  this is the only necessary config to discover the templates for each of the scenarios listed above.
- `new_environment_template_source`: Git URL OR local path to the template source for the `new environment` scenario.
  If it is a relative path, will assume it is relative to the `infrastructure_templates_repo`. This can be used to
  override the location of the template, as an alternative to the convention followed in `infrastructure-templates`, or
  even source from a completely different repository.
- `new_region_template_source`, `new_account_template_source`, `new_wrapper_module_template_source`: Similar to
  `new_environment_template_source`, but for each of the other scenarios that `terragrunt new` supports.

### Template repository structure

The `new` command will assume the following template repository structure:

```
infrastructure-templates-example
├── CONTRIBUTING.md
├── LICENSE.txt
├── README.md
├── templates
│   ├── account/
│   ├── component/
│   ├── environment/
│   ├── region/
│   └── wrapper-module/
└── test
```

Where `templates` and `test` are directories that contain the template folders and corresponding tests, respectively.

By convention, templates in the `infrastructure-templates` repo with the following names will automatically be used for
each of the `new` command use cases:

- `component` => `terragrunt new component`
- `environment` => `terragrunt new environment`
- `region` => `terragrunt new region`
- `account` => `terragrunt new account`
- `wrapper-module` => `terragrunt new wrapper-module`

Note that the templates folder is not restricted to just these 4 templates: you are free to define as many or as little
templates as you need. Additional templates that do not fit the convention can be invoked using `terragrunt new
unicorn`.

### Template engine

Templates are written and rendered using the default Go Template engine. While there are many other template engines to
consider, given that the templates are rendered using Terragrunt as a frontend, it makes the most sense to use the built
in templating engine of Go.

**Pros**:

- Go templating is builtin to Go, and thus many Go tools use this (e.g `hugo`, `helm`). This means that it will be
  familiar with the Go community.
- There are lots of utilities we can use for improving the experience of working with Go Templates, such as
  [Sprig](http://masterminds.github.io/sprig/).
- Does not depend on external tools: can be builtin to `terragrunt`, and thus is portable.

**Cons**:

- Go templating is not necessary a good language to work with. You lose editor support, and some of the constructs are
  not intuitive (E.g whitespace chomping).
- Requires some work to embed into `terragrunt` so that arbitrary templates can be invoked.
- We need to invent a DSL for handling and declaring template inputs.

### Template tests

Template tests can be invoked using a new module in `terratest` that can be used to call `terragrunt new` using these
templates with arbitrary input values. You can then apply the generated template to test the generated code against
various variable inputs.

### Template structure

_TODO: This needs to be fleshed out. Note that this is conditional on if the above proposal is accepted._

### FAQ

#### When should code be generated?

_TODO_

#### Should the generated code be checked in?

_TODO_

#### How to keep code in sync with template?

_TODO_

#### Use Case: Is there a way to continuously generate code from a template?

_TODO_


## Alternatives

#### Third party scaffolding tool

Instead of baking in scaffolding into Terragrunt, we can rely on using other tools for templating terraform code. There
are many tools that attempt to solve this issue of scaffolding code to bootstrap new projects:

- [cookiecutter](https://github.com/cookiecutter/cookiecutter): python based tool that uses jinja2 for templating
- [yeoman](https://yeoman.io/): nodejs templating tool
- [kapitan](https://github.com/deepmind/kapitan): python based tool that supports both jsonnet and jinja2
- [giter8](templatin://github.com/foundweekends/giter8): scala based tool for generating project templates

We could pick one of these tools and provide templates based on that tool, with instructions for which templates to use
for generating the code for the scenario.

**Pros**:

- Keeps Terragrunt clean, since no new code is introduced.
- Feature is opt in: you can choose not to install the tool.
- If we pick a popular and well maintained library, we could get lots of useful features out of the box.
- If we pick a popular and well maintained library, learning that tool could be useful for other use cases beyond
  Terragrunt/Terraform codegen (Knowledge is Transferrable).

**Cons**:

- Having to install another tool to get up and running is not very user friendly. Ideally, the user should only have to
  interact with Terragrunt to do things with Terragrunt; it feels awkward to transition to another tool to scaffold
  Terragrunt config.
- We have to be aware of platform compatibility. For example, `cookiecutter` and `kapitan` depend on python, and
  installation will install different versions of their dependencies that can mess up a users' machine. As such, they
  will need to setup a virtualenv for maximum safety, which adds complexity.
- We can't optimize the UX for Terragrunt. Since the tool lives outside Terragrunt, we can't be opinionated about how we
  call the scaffolds for each of the use case scenarios to provide a better interactive experience.

#### Terragrunt Wrapper for Scaffolding tool

This approach is an enhancement to the [third party scaffolding tool](#third-party-scaffolding-tool) approach, but
instead of directly using the tool, we wrap the tool in Terragrunt.

**Pros**:

- Allows opportunity to optimize the templating UX for Terragrunt.
- Feature is still opt in. You don't need to install the underlying scaffolding tool if you don't need to use the
  feature.

**Cons**:

- Adds a layer of indirection which can be confusing
    - User has to install a tool that they never call directly.
    - Not only is the user learning a new scaffolding tool, they have to also learn how Terragrunt uses it and how to
      configure Terragrunt to use it.

#### Alternative Template Engines

In the community, there are [a ton of templating engines](https://github.com/avelino/awesome-go#template-engines)
available for golang beyond the default one available in the standard library. That said, many of these have not been
updated in years and are relatively small in scope.

The only one that comes remotely close in terms of feature set is [pongo2](https://github.com/flosch/pongo2), which
provides [jinja2](https://palletsprojects.com/p/jinja/) template syntax for Go. That said, there doesn't seem to be any
real advantage to using `pongo2` over the existing template engine, other than it being more familiar to people
migrating over from the Python community.


## References

-
