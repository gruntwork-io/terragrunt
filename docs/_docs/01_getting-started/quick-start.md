---
layout: collection-browser-doc
title: Quick Start
category: getting-started
excerpt: Start using Terragrunt today!
tags: ["tofu", "opentofu", "terraform", "tf"]
order: 101
nav_title: Documentation
nav_title_link: /docs/
---

## Install Terragrunt

If you haven't already installed Terragrunt, you can do so by following the instructions in the [Install Terragrunt]({{site.baseurl}}/docs/getting-started/install/) guide.

## Add `terragrunt.hcl` to your project

If you are currently using OpenTofu or Terraform, and you want to start using Terragrunt in your project, simply run the following where your OpenTofu project is located:

```shell
touch terragrunt.hcl
```

This creates an empty Terragrunt configuration file in the directory where you are using OpenTofu. You can now start using `terragrunt` instead of `tofu` or `terraform` to run your OpenTofu/Terraform commands as if you were simply using OpenTofu or Terraform.

Depending on why you're looking to adopt Terragrunt, this may be all you need to do!

With just this empty file, you've already made it so that you no longer need to run `tofu init` or `terraform init` before running `tofu apply` or `terraform apply`. Terragrunt will automatically run `init` for you if necessary. This is a feature called [Auto-init](/docs/features/auto-init/).

This might not be very impressive so far, so you may be wondering _why_ one might want to start using Terragrunt to manage their OpenTofu/Terraform projects. The next section will give you a very gentle introduction to using Terragrunt, and show you how you can start to leverage Terragrunt to manage your OpenTofu/Terraform projects more effectively.

## Tutorial

What follows is a gentle step-by-step guide to integrating Terragrunt into a new (or existing) OpenTofu/Terraform project.

For the sake of this tutorial, a minimal set of OpenTofu configurations will be used so that you can follow along. Following these steps will give you an idea of how to integrate Terragrunt into an existing project, even if yours is more complex.

This tutorial will assume the following:

1. You have [OpenTofu](https://opentofu.org/docs/intro/install/) or [Terraform](https://developer.hashicorp.com/terraform/install) installed\*.
2. You have a basic understanding of what OpenTofu/Terraform do.
3. You are using a Unix-like operating system.

This tutorial will not assume the following:

1. You have any subscriptions to any cloud providers.
2. You have any experience with Terragrunt.
3. You have any existing Terragrunt, OpenTofu or Terraform projects.

\* Note that if you have _both_ OpenTofu and Terraform installed, you'll want to read the [terragrunt-tfpath](/docs/reference/cli-options/#terragrunt-tfpath) docs to understand how Terragrunt determines which binary to use.

If you would like a less gentle introduction geared towards users with an active AWS account, familiarity with OpenTofu/Terraform, and potentially a team actively using Terragrunt, consider starting with the [Overview](/docs/getting-started/overview/).

If you start to feel lost, or don't understand a concept, consider reading the [Terminology](/docs/getting-started/terminology/) page before continuing with this tutorial. It has a brief overview of most of the common terms used when discussing Terragrunt.

Finally, note that all of the files created in this tutorial can be copied directly from the code block, none of them are partial files, so you don't have to worry about figuring out where to put the code. Just copy and paste!

You can also see what to expect in your filesystem at each step [here](https://github.com/gruntwork-io/terragrunt/tree/main/test/fixtures/docs/01-quick-start).
<!-- Maintainer's Note: we also test this continuously in `tests/integration_docs_test.go` -->

### Step 1: Create a new Terragrunt project

Let's say you have the following `main.tf` in directory `foo`:

```hcl
# foo/main.tf
resource "local_file" "file" {
  content  = "Hello, World!"
  filename = "${path.module}/hi.txt"
}
```

As we learned above, integrating this OpenTofu project with Terragrunt is as simple as creating a `terragrunt.hcl` file in the same directory:

```bash
touch foo/terragrunt.hcl
```

You can now run `terragrunt` commands within the `foo` directory, as if you were using `tofu` or `terraform`.

```bash
$ cd foo
$ terragrunt apply -auto-approve
18:44:26.066 STDOUT tofu: Initializing the backend...
18:44:26.067 STDOUT tofu: Initializing provider plugins...
18:44:26.067 STDOUT tofu: - Finding latest version of hashicorp/local...
18:44:26.717 STDOUT tofu: - Installing hashicorp/local v2.5.2...
18:44:27.033 STDOUT tofu: - Installed hashicorp/local v2.5.2 (signed, key ID 0C0AF313E5FD9F80)
18:44:27.033 STDOUT tofu: Providers are signed by their developers.
18:44:27.033 STDOUT tofu: If you'd like to know more about provider signing, you can read about it here:
18:44:27.033 STDOUT tofu: https://opentofu.org/docs/cli/plugins/signing/
18:44:27.034 STDOUT tofu: OpenTofu has created a lock file .terraform.lock.hcl to record the provider
18:44:27.034 STDOUT tofu: selections it made above. Include this file in your version control repository
18:44:27.034 STDOUT tofu: so that OpenTofu can guarantee to make the same selections by default when
18:44:27.034 STDOUT tofu: you run "tofu init" in the future.
18:44:27.034 STDOUT tofu: OpenTofu has been successfully initialized!
18:44:27.035 STDOUT tofu:
18:44:27.035 STDOUT tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
18:44:27.035 STDOUT tofu: any changes that are required for your infrastructure. All OpenTofu commands
18:44:27.035 STDOUT tofu: should now work.
18:44:27.035 STDOUT tofu: If you ever set or change modules or backend configuration for OpenTofu,
18:44:27.035 STDOUT tofu: rerun this command to reinitialize your working directory. If you forget, other
18:44:27.035 STDOUT tofu: commands will detect it and remind you to do so if necessary.
18:44:27.362 STDOUT tofu: OpenTofu used the selected providers to generate the following execution
18:44:27.362 STDOUT tofu: plan. Resource actions are indicated with the following symbols:
18:44:27.362 STDOUT tofu:   + create
18:44:27.362 STDOUT tofu: OpenTofu will perform the following actions:
18:44:27.362 STDOUT tofu:   # local_file.file will be created
18:44:27.362 STDOUT tofu:   + resource "local_file" "file" {
18:44:27.362 STDOUT tofu:       + content              = "Hello, World!"
18:44:27.362 STDOUT tofu:       + content_base64sha256 = (known after apply)
18:44:27.362 STDOUT tofu:       + content_base64sha512 = (known after apply)
18:44:27.362 STDOUT tofu:       + content_md5          = (known after apply)
18:44:27.362 STDOUT tofu:       + content_sha1         = (known after apply)
18:44:27.362 STDOUT tofu:       + content_sha256       = (known after apply)
18:44:27.362 STDOUT tofu:       + content_sha512       = (known after apply)
18:44:27.362 STDOUT tofu:       + directory_permission = "0777"
18:44:27.362 STDOUT tofu:       + file_permission      = "0777"
18:44:27.362 STDOUT tofu:       + filename             = "./hi.txt"
18:44:27.362 STDOUT tofu:       + id                   = (known after apply)
18:44:27.362 STDOUT tofu:     }
18:44:27.362 STDOUT tofu: Plan: 1 to add, 0 to change, 0 to destroy.
18:44:27.362 STDOUT tofu:
18:44:27.383 STDOUT tofu: local_file.file: Creating...
18:44:27.384 STDOUT tofu: local_file.file: Creation complete after 0s [id=0a0a9f2a6772942557ab5355d76af442f8f65e01]
18:44:27.392 STDOUT tofu:
18:44:27.392 STDOUT tofu: Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
18:44:27.392 STDOUT tofu:
```

You might notice that this is a little more verbose than the output you're used to seeing from running `tofu` or `terraform` directly. This is because Terragrunt does a bit of work behind the scenes to make sure that you can scale your OpenTofu/Terraform usage without running into common problems. As you get more comfortable with using Terragrunt on larger projects, you may find the extra information helpful.

If you would prefer that Terragrunt output look more like the output from `tofu` or `terraform`, you can use the `--terragrunt-log-format bare` flag (or set the environment variable `TERRAGRUNT_LOG_FORMAT=bare`) to reduce the verbosity of the output.

e.g.

```bash
$ terragrunt --terragrunt-log-format bare apply
local_file.file: Refreshing state... [id=0a0a9f2a6772942557ab5355d76af442f8f65e01]

No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.

Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
```

The way dynamicity is handled in OpenTofu is via `variable` configuration blocks. Let's add one to our `main.tf` so that we can control the content of the file we're creating:

```hcl
# foo/main.tf
variable "content" {}

resource "local_file" "file" {
  content  = var.content
  filename = "${path.module}/hi.txt"
}
```

Now, just like when using `tofu` alone, you can pass in the value for the `content` variable using the `-var` flag:

```bash
terragrunt apply -auto-approve -var content='Hello, Terragrunt!'
```

This is a common pattern when working with Infrastructure as Code (IaC). You typically create IaC that is relatively static, and then as you need to make configurations dynamic, you add variables to your configuration files to introduce dynamicity.

### Step 2: Add a new Terragrunt unit

In the context of Terragrunt, a "unit" is a directory that contains a `terragrunt.hcl` file, and it represents a single piece of infrastructure. You can think of a unit as a single instance of an OpenTofu/Terraform module.

Let's create a copy of the `foo` directory and call it `bar`:

```bash
cd ..
cp -r foo bar
```

We now have two identical units in our project, `foo` and `bar`. We also have identical code in each of these directories, which is not ideal if we want to be able to avoid duplicating effort when we make changes to our infrastructure.

### Step 3: Create a shared module

To avoid this duplication, we can introduce a new `shared` directory, and reference that directory from both `foo` and `bar`. This way, we can make changes to our infrastructure in one place and have those changes apply to both units.

Let's create a new directory called `shared`:

```bash
mkdir shared
```

Now, move the `main.tf` file from `foo` to `shared`:

```bash
mv foo/main.tf shared/main.tf
```

Finally, let's update the `foo` and `bar` directories to reference the `shared` directory. Update the `main.tf` files in both `foo` and `bar` to look like this:

```hcl
# foo/main.tf and bar/main.tf
variable "content" {}

module "shared" {
  source = "../shared"

  content = var.content
}
```

There's now one place where the logic for the resource `local_file.file` is defined, and both `foo` and `bar` reference that logic. You can imagine that as your infrastructure grows, it can become more and more advantageous to put repeated logic into shared modules like this.

This setup does have some problems, however. While you could keep navigating to the different units and running `terragrunt apply` in each one with the appropriate `-var` flags, this can quickly become tedious, as you have to know which units require which set of vars applied. You might decide to work around this by creating a file named `terraform.tfvars` in each unit directory, but this also comes with some limitations that Terragrunt can help you avoid.

### Step 4: Use Terragrunt to manage your units

Luckily, Terragrunt has a built-in feature to control the inputs passed to your OpenTofu/Terraform configurations. This feature is called (aptly enough) [inputs](/docs/reference/config-blocks-and-attributes/#inputs).

Let's add inputs to both `terragrunt.hcl` files in the `foo` and `bar` directories:

```hcl
# foo/terragrunt.hcl
inputs = {
  content = "Hello from foo, Terragrunt!"
}
```

```hcl
# bar/terragrunt.hcl
inputs = {
  content = "Hello from bar, Terragrunt!"
}
```

You don't have to maintain the extra `main.tf` files just to instantiate the `module` blocks. You can use the `terraform` block to handle this for you. Update the `terragrunt.hcl` files in `foo` and `bar` to look like this:

```hcl
# foo/terragrunt.hcl
terraform {
  source = "../shared"
}

inputs = {
  content = "Hello from foo, Terragrunt!"
}
```

```hcl
# bar/terragrunt.hcl
terraform {
  source = "../shared"
}

inputs = {
  content = "Hello from bar, Terragrunt!"
}
```

And you can delete the `main.tf` files from both `foo` and `bar`:

```bash
rm foo/main.tf bar/main.tf
```

This saves you some duplicated content, as you no longer need to maintain that extra `content` variable in each `main.tf` file. You can imagine that for especially large modules, the ability to define inputs in the `terragrunt.hcl` file can save you a lot of time and effort. The patterns for your infrastructure are exclusively defined in `.tf` files now, and the `terragrunt.hcl` files are used to manage the instances of those patterns as units.

If you run `terragrunt apply -auto-approve` in the `foo` and `bar` directories, you'll see that the `content` variable is set to the value you defined in the `inputs` block of the `terragrunt.hcl` file. You might also notice that there's now a special `.terragrunt-cache` directory generated for you in each unit directory. This is where Terragrunt copies the contents of modules, and performs any necessary additional code generation to make sure that your OpenTofu/Terraform code is ready to be run.

The `.terragrunt-cache` directory is typically added to `.gitignore` files, similar to the `.terraform` directory that OpenTofu generates.

### Step 5: Use Terragrunt to manage your stacks

In the context of Terragrunt, a "stack" is a collection of units that are managed together. You can think of a stack as a single environment, such as `dev`, `staging`, or `prod`, or an entire project.

One of the main reasons users adopt Terragrunt is how it can help manage the complexity of managing multiple units across multiple environments.

e.g. Let's say we wanted to update both our `foo` and `bar` environments simultaneously.

In the directory above `foo` and `bar`, run the following:

```bash
$ terragrunt run-all apply -auto-approve
08:42:00.150 INFO   The stack at . will be processed in the following order for command apply:
Group 1
- Module ./bar
- Module ./foo


Are you sure you want to run 'terragrunt apply' in each folder of the stack described above? (y/n) y
08:43:10.702 STDOUT [foo] tofu: local_file.file: Refreshing state... [id=c4ae21736a6297f44ea86791e528338e9d14a0e9]
08:43:10.702 STDOUT [bar] tofu: local_file.file: Refreshing state... [id=f855394a0316da09618c8b1fde7b91e00e759f80]
08:43:10.708 STDOUT [bar] tofu: No changes. Your infrastructure matches the configuration.
08:43:10.708 STDOUT [bar] tofu: OpenTofu has compared your real infrastructure against your configuration and
08:43:10.708 STDOUT [bar] tofu: found no differences, so no changes are needed.
08:43:10.708 STDOUT [foo] tofu: No changes. Your infrastructure matches the configuration.
08:43:10.708 STDOUT [foo] tofu: OpenTofu has compared your real infrastructure against your configuration and
08:43:10.708 STDOUT [foo] tofu: found no differences, so no changes are needed.
08:43:10.716 STDOUT [foo] tofu:
08:43:10.716 STDOUT [foo] tofu: Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
08:43:10.716 STDOUT [foo] tofu:
08:43:10.720 STDOUT [bar] tofu:
08:43:10.720 STDOUT [bar] tofu: Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
08:43:10.720 STDOUT [bar] tofu:
```

This is where that additional verbosity in Terragrunt logging is really handy. You can see that Terragrunt concurrently ran `apply -auto-approve` in both the `foo` and `bar` units. The extra logging for Terragrunt also included information on the names of the units that were processed, and disambiguated the output from each unit.

Similar to the `tofu` CLI, there is a prompt to confirm that you are sure you want to run the command in each unit when performing a command that's potentially destructive. You can skip this prompt by using the `--terragrunt-non-interactive` flag, just as you would with `-auto-approve` in OpenTofu.

```bash
terragrunt run-all --terragrunt-non-interactive apply -auto-approve
```

### Step 6: Use Terragrunt to manage your DAG

In the context of Terragrunt, a Directed Acyclic Graph (DAG) that represents the graph units in your stack, determined by their dependencies. Terragrunt uses the DAG to determine the order in which it performs runs across your stack.

For example, let's say that the `content` of the `bar` unit depended on the `content` of the `foo` unit. You can express this dependency first by adding an `output` block to the `shared` module:

```hcl
# shared/output.tf
output "content" {
  value = local_file.file.content
}
```

Then, you can update the `bar` unit to depend on the `foo` unit by using the `dependencies` block in the `terragrunt.hcl` file:

```hcl
# bar/terragrunt.hcl
terraform {
 source = "../shared"
}

dependency "foo" {
 config_path = "../foo"
}

inputs = {
 content = "Foo content: ${dependency.foo.outputs.content}"
}
```

Being good citizens of the IaC world, we should run a `plan` before an `apply` to see what changes Terragrunt will make to our infrastructure (note that you will get an error here. This is expected, and we'll fix it in the next step):

```bash
$ terragrunt run-all plan
08:57:09.271 INFO   The stack at . will be processed in the following order for command plan:
Group 1
- Module ./foo

Group 2
- Module ./bar

...

08:57:09.936 ERROR  [bar] Module ./bar has finished with an error
08:57:09.936 ERROR  error occurred:

* ./foo/terragrunt.hcl is a dependency of ./bar/terragrunt.hcl but detected no outputs. Either the target module has not been applied yet, or the module has no outputs. If this is expected, set the skip_outputs flag to true on the dependency block.

08:57:09.936 ERROR  Unable to determine underlying exit code, so Terragrunt will exit with error code 1
```

Oh no! We got an error. This happens because the way in which dependencies are resolved by default in Terragrunt is to run `terragrunt output` within the dependency for use in the dependent unit. In this case, the `foo` unit has not been applied yet, so there are no outputs to fetch.

You should notice, however, that Terragrunt has already figured out the order in which to run the `plan` command across the units in your stack. This is what we mean when we say that Terragrunt uses a DAG to determine the order in which to run commands across your stack. Terragrunt analyzes the dependencies across your units, and determines the order for runs so that outputs are ready to be used as inputs in dependent units.

If you instead decided to run `terragrunt run-all apply -auto-approve`, you would instead see Terragrunt complete the `apply` in the `foo` unit first, and then complete the `apply` in the `bar` unit, as it's aware that the `bar` unit might need some outputs from the `foo` unit.

### Step 7: Use mocks to handle unavailable outputs

In this scenario, most Terragrunt users leverage `mock_outputs` to handle unavailable outputs (see [limitations on accessing exposed config](https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes/#limitations-on-accessing-exposed-config)). Given that it's expected that the `foo` unit won't be able to provide outputs until it's applied, you can use the `mock_outputs` block to provide a placeholder value for the `content` output during the `plan` phase.

```hcl
# bar/terragrunt.hcl
terraform {
  source = "../shared"
}

dependency "foo" {
  config_path = "../foo"
  mock_outputs = {
    content = "Mocked content from foo"
  }
}

inputs = {
  content = "Foo content: ${dependency.foo.outputs.content}"
}
```

Re-running the `plan` command should now complete successfully:

```bash
$ terragrunt run-all plan
09:29:03.461 INFO   The stack at . will be processed in the following order for command plan:
Group 1
- Module ./foo

Group 2
- Module ./bar

...

09:29:03.644 WARN   [bar] Config ./foo/terragrunt.hcl is a dependency of ./bar/terragrunt.hcl that has no outputs, but mock outputs provided and returning those in dependency output.

...

09:29:03.898 STDOUT [bar] tofu:   + resource "local_file" "file" {
09:29:03.898 STDOUT [bar] tofu:       + content              = "Foo content: Mocked content from foo"
09:29:03.898 STDOUT [bar] tofu:       + content_base64sha256 = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + content_base64sha512 = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + content_md5          = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + content_sha1         = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + content_sha256       = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + content_sha512       = (known after apply)
09:29:03.898 STDOUT [bar] tofu:       + directory_permission = "0777"
09:29:03.898 STDOUT [bar] tofu:       + file_permission      = "0777"
09:29:03.898 STDOUT [bar] tofu:       + filename             = "./hi.txt"
09:29:03.898 STDOUT [bar] tofu:       + id                   = (known after apply)
09:29:03.898 STDOUT [bar] tofu:     }
```

If you're concerned about the `mock_outputs` attribute resulting in invalid configurations, note that during an apply, the outputs of `foo` will be known, and Terragrunt won't use `mock_outputs` to resolve the outputs of `foo`.

```bash
$ terragrunt run-all --terragrunt-non-interactive apply -auto-approve

...

09:31:21.587 STDOUT [bar] tofu:   + resource "local_file" "file" {
09:31:21.587 STDOUT [bar] tofu:       + content              = "Foo content: Hello from foo, Terragrunt!"
09:31:21.587 STDOUT [bar] tofu:       + content_base64sha256 = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + content_base64sha512 = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + content_md5          = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + content_sha1         = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + content_sha256       = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + content_sha512       = (known after apply)
09:31:21.587 STDOUT [bar] tofu:       + directory_permission = "0777"
09:31:21.587 STDOUT [bar] tofu:       + file_permission      = "0777"
09:31:21.587 STDOUT [bar] tofu:       + filename             = "./hi.txt"
09:31:21.587 STDOUT [bar] tofu:       + id                   = (known after apply)
09:31:21.587 STDOUT [bar] tofu:     }

...
```

You can also be explicit about the fact that you only want to use `mock_outputs` during the `plan` phase by specifying that in your `mock_outputs` configuration:

```hcl
# bar/terragrunt.hcl
terraform {
  source = "../shared"
}

dependency "foo" {
  config_path = "../foo"
  mock_outputs = {
    content = "Mocked content from foo"
  }

  mock_outputs_allowed_terraform_commands = ["plan"]
}

inputs = {
  content = "Foo content: ${dependency.foo.outputs.content}"
}
```

Something a little subtle just happened there. Note that the `inputs` attribute is dynamic. This addresses some of the limitations mentioned earlier about using `terraform.tfvars` files to manage inputs for units. Given that the `bar` unit is dependent on output values from the `foo` unit, you wouldn't be able to use a `terraform.tfvars` file to populate this variable without some additional tooling to populate it dynamically.

Terragrunt was spawned organically out of supporting Gruntwork customers using Terraform at scale, and features in the product are designed to address common problems like these that arise when managing OpenTofu/Terraform projects at scale in production.

### Step 8: Continue learning and exploring

Hopefully, following this simple tutorial has given you confidence in integrating Terragrunt into your existing OpenTofu/Terraform projects. Starting small, and gradually introducing more complex Terragrunt features is a great way to learn how Terragrunt can help you manage your infrastructure more effectively.

The next step of the Getting Started guide is to follow the [Overview](/docs/getting-started/overview/) guide. This guide will introduce you to more advanced Terragrunt features, and show you how to use Terragrunt to manage your infrastructure across multiple environments in a real-world AWS account.

If you're ready to get your hands dirty with more advanced Terragrunt features yourself, you can skip ahead to the [Features](/docs#features) section of the documentation.

If you ever need help with a particular problem, take a look at the resources available to you in the [Support](/docs/community/support/) section. You are especially encouraged to join the [Terragrunt Discord](https://discord.gg/SPu4Degs5f) server, and become part of the Terragrunt community.
