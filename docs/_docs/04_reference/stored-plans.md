---
layout: collection-browser-doc
title: Stored Plans
category: reference
categories_url: reference
excerpt: Learn how to work with stored plans in Terragrunt.
tags: ["plan"]
order: 410
nav_title: Documentation
nav_title_link: /docs/
---

The [-out](https://opentofu.org/docs/cli/commands/plan/) flag of the OpenTofu/Terraform plan command is powerful.

It allows you to take an OpenTofu/Terraform plan and store the result as a binary file on disk.

What makes this so powerful is that you can then use that binary file to do a number of things like:

1. Reproduce the plan stdout exactly as it was when the plan was run.
2. Apply the plan, only making the changes that were explicitly in that plan.
3. Programmatically parse the plan to extract metadata from it.

This is especially useful when you want to store the plan output for use later on, like when working in a CI/CD pipeline.

Unfortunately, a lot of what makes Terragrunt powerful also introduces additional complexity when working with stored plans. This guide will help you understand how to work with stored plans in Terragrunt.

## Starting Small

Let's start with a simple example. Here's the simplest way you can create a Terragrunt unit that stores a plan:

```bash
mkdir -p example
cd example
touch terragrunt.hcl
cat <<EOF > main.tf
resource "null_resource" "example" {
  triggers = {
    always_run = timestamp()
  }
}
EOF

```

Now, run a plan, and save it using the `-out` flag:

```bash
$ terragrunt plan -out plan.out
16:59:34.913 INFO   tofu: Initializing the backend...
16:59:34.913 INFO   tofu: Initializing provider plugins...
16:59:34.913 INFO   tofu: - Finding latest version of hashicorp/null...
16:59:35.140 INFO   tofu: - Installing hashicorp/null v3.2.3...
16:59:35.422 INFO   tofu: - Installed hashicorp/null v3.2.3 (signed, key ID 0C0AF313E5FD9F80)
16:59:35.422 INFO   tofu: Providers are signed by their developers.
16:59:35.422 INFO   tofu: If you'd like to know more about provider signing, you can read about it here:
16:59:35.422 INFO   tofu: https://opentofu.org/docs/cli/plugins/signing/
16:59:35.422 INFO   tofu: OpenTofu has created a lock file .terraform.lock.hcl to record the provider
16:59:35.422 INFO   tofu: selections it made above. Include this file in your version control repository
16:59:35.422 INFO   tofu: so that OpenTofu can guarantee to make the same selections by default when
16:59:35.422 INFO   tofu: you run "tofu init" in the future.
16:59:35.422 INFO   tofu: OpenTofu has been successfully initialized!
16:59:35.422 INFO   tofu:
16:59:35.422 INFO   tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
16:59:35.422 INFO   tofu: any changes that are required for your infrastructure. All OpenTofu commands
16:59:35.422 INFO   tofu: should now work.
16:59:35.422 INFO   tofu: If you ever set or change modules or backend configuration for OpenTofu,
16:59:35.422 INFO   tofu: rerun this command to reinitialize your working directory. If you forget, other
16:59:35.422 INFO   tofu: commands will detect it and remind you to do so if necessary.
16:59:35.780 STDOUT tofu: OpenTofu used the selected providers to generate the following execution
16:59:35.780 STDOUT tofu: plan. Resource actions are indicated with the following symbols:
16:59:35.780 STDOUT tofu:   + create
16:59:35.780 STDOUT tofu: OpenTofu will perform the following actions:
16:59:35.780 STDOUT tofu:   # null_resource.example will be created
16:59:35.780 STDOUT tofu:   + resource "null_resource" "example" {
16:59:35.780 STDOUT tofu:       + id       = (known after apply)
16:59:35.780 STDOUT tofu:       + triggers = {
16:59:35.780 STDOUT tofu:           + "always_run" = (known after apply)
16:59:35.780 STDOUT tofu:         }
16:59:35.780 STDOUT tofu:     }
16:59:35.780 STDOUT tofu: Plan: 1 to add, 0 to change, 0 to destroy.
16:59:35.780 STDOUT tofu:
16:59:35.780 STDOUT tofu:
16:59:35.780 STDOUT tofu: ─────────────────────────────────────────────────────────────────────────────
16:59:35.780 STDOUT tofu: Saved the plan to: plan.out
16:59:35.780 STDOUT tofu: To perform exactly these actions, run the following command to apply:
16:59:35.780 STDOUT tofu:     tofu apply "plan.out"
```

You now have a binary file called `plan.out` that contains your plan.

You can simply reproduce the plan stdout exactly as it was when the plan was run using the `show` command:

```bash
$ terragrunt show plan.out
17:01:59.406 STDOUT tofu: OpenTofu used the selected providers to generate the following execution
17:01:59.406 STDOUT tofu: plan. Resource actions are indicated with the following symbols:
17:01:59.406 STDOUT tofu:   + create
17:01:59.406 STDOUT tofu: OpenTofu will perform the following actions:
17:01:59.406 STDOUT tofu:   # null_resource.example will be created
17:01:59.406 STDOUT tofu:   + resource "null_resource" "example" {
17:01:59.406 STDOUT tofu:       + id       = (known after apply)
17:01:59.406 STDOUT tofu:       + triggers = {
17:01:59.406 STDOUT tofu:           + "always_run" = (known after apply)
17:01:59.406 STDOUT tofu:         }
17:01:59.406 STDOUT tofu:     }
17:01:59.406 STDOUT tofu: Plan: 1 to add, 0 to change, 0 to destroy.
17:01:59.406 STDOUT tofu:
```

You can also apply it using the `apply` command:

```bash
$ terragrunt apply plan.out
16:45:21.818 INFO   tofu: Initializing the backend...
16:45:21.818 INFO   tofu: Initializing provider plugins...
16:45:21.818 INFO   tofu: OpenTofu has been successfully initialized!
16:45:21.818 INFO   tofu:
16:45:21.818 INFO   tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
16:45:21.818 INFO   tofu: any changes that are required for your infrastructure. All OpenTofu commands
16:45:21.818 INFO   tofu: should now work.
16:45:21.818 INFO   tofu: If you ever set or change modules or backend configuration for OpenTofu,
16:45:21.818 INFO   tofu: rerun this command to reinitialize your working directory. If you forget, other
16:45:21.820 INFO   tofu: commands will detect it and remind you to do so if necessary.
16:45:21.846 STDOUT tofu:
16:45:21.846 STDOUT tofu: Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
16:45:21.846 STDOUT tofu:
```

Finally, you can also programmatically parse the plan to extract metadata from it:

```bash
$ terragrunt --terragrunt-log-disable show -json plan.out | jq
{
  "format_version": "1.2",
  "terraform_version": "1.8.5",
  "planned_values": {
    "root_module": {}
  },
  "configuration": {
    "root_module": {}
  },
  "timestamp": "2024-12-20T21:43:40Z",
  "errored": false
}
```

Note that I used the `--terragrunt-log-disable` flag to disable Terragrunt logging for the sake of clarity, and the `jq` command to pretty-print the JSON output.
