---
layout: collection-browser-doc
title: Debugging
category: features
categories_url: features
excerpt: Learn how to debug issues with terragrunt and terraform.
tags: ["DRY", "Use cases", "CLI"]
order: 265
nav_title: Documentation
nav_title_link: /docs/
---

## Debugging

Terragrunt and Terraform usually play well together in helping you
write DRY, re-usable infrastructure. But how do we figure out what
went wrong in the rare case that they _don't_ play well?

Terragrunt provides a way to configure logging level through the `--terragrunt-log-level`
command flag. Additionally, Terragrunt provides `--terragrunt-debug`, that can be used
to generate `terragrunt-debug.tfvars.json`.

For example you could use it like this to debug an `apply` that's producing
unexpected output:

    $ terragrunt apply --terragrunt-log-level debug --terragrunt-debug

Running this command will do two things for you:
  - Output a file named `terragrunt-debug.tfvars.json` to your terragrunt working
    directory (the same one containing your `terragrunt.hcl`)
  - Print instructions on how to invoke terraform against the generated file to
    reproduce exactly the same terraform output as you saw when invoking
    `terragrunt`. This will help you to determine where the problem's root cause
    lies.

Using those features is helpful when you want determine which of these three major areas is the
root cause of your problem:
  1. Misconfiguration of your infrastructure code.
  2. An error in `terragrunt`.
  3. An error in `terraform`.

Let's run through a few use-cases.

### Use-case: I use locals or dependencies in terragrunt.hcl, and the terraform output isn't what I expected

Consider this file structure for a fictional production environment where we
have configured an application to deploy as many tasks as there are minimum
number of machines in some cluster.

```
    └── live
        └── prod
            └── app
            |   ├── vars.tf
            |   ├── main.tf
            |   ├── outputs.tf
            |   └── terragrunt.hcl
            └── ecs-cluster
                └── outputs.tf
```

The files contain this text (`app/main.tf` and `ecs-cluster/outputs.tf` omitted
for brevity):

```hcl
# app/vars.tf
variable "image_id" {
  type = string
}

variable "num_tasks" {
  type = number
}

# app/outputs.tf
output "task_ids" {
  value = module.app_infra_module.task_ids
}

# app/terragrunt.hcl
locals {
  image_id = "acme/myapp:1"
}

dependency "cluster" {
  config_path = "../ecs-cluster"
}

inputs = {
  image_id = locals.image_id
  num_tasks = dependency.cluster.outputs.cluster_min_size
}
```

You perform a `terragrunt apply`, and find that `outputs.task_ids` has 7
elements, but you know that the cluster only has 4 VMs in it! What's happening?
Let's figure it out. Run this:

    $ terragrunt apply --terragrunt-log-level debug --terragrunt-debug

After applying, you will see this output on standard error

```
[terragrunt] Variables passed to terraform are located in "~/live/prod/app/terragrunt-debug.tfvars"
[terragrunt] Run this command to replicate how terraform was invoked:
[terragrunt]     terraform apply -var-file="~/live/prod/app/terragrunt-debug.tfvars.json" "~/live/prod/app"
```

Well we may have to do all that, but first let's just take a look at `terragrunt-debug.tfvars.json`

```hcl
{
    "image_id": "acme/myapp:1",
    "num_tasks": 7
}
```

So this gives us the clue -- we expected `num_tasks` to be 4, not 7! Looking into
`ecs-cluster/outputs.tf` we see this text:

```hcl
# ecs-cluster/outputs.tf
output "cluster_min_size" {
  value = module.my_cluster_module.cluster_max_size
}
```

Oops! It says `max` when it should be `min`. If we fix `ecs-cluster/outputs.tf`
we should be golden! We fix the problem in time to take a nice afternoon walk in
the sun.

In this example we've seen how debug options can help us root cause issues
in dependency and local variable resolution.

<!-- See
https://github.com/gruntwork-io/terragrunt/blob/eb692a83bee285b0baaaf4b271c66230f99b6358/docs/_docs/02_features/debugging.md
for thoughts on other potential features to implement.
-->
