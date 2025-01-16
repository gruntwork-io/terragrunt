---
layout: collection-browser-doc
title: Logging
category: reference
categories_url: reference
excerpt: Learn how Terragrunt decides what to log, and when.
tags: ["log"]
order: 408
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt logs messages as it runs to help you understand what it's doing. Given that Terragrunt is an IaC orchestrator, this can result in messages that are surprising if you don't understand what Terragrunt is doing behind the scenes.

## Log Levels

To start with, Terragrunt has the following log levels:

- `STDERR`
- `STDOUT`
- `ERROR`
- `WARN`
- `INFO`
- `DEBUG`
- `TRACE`

The `STDOUT` and `STDERR` log levels are non-standard, and exist due to Terragrunt's special responsibility as an IaC orchestrator.

For the most part, whenever you use Terragrunt to run something using another tool (like OpenTofu or Terraform), Terragrunt will capture the stdout and stderr terminal output from that tool, enrich it with additional information, then _log_ it as `STDOUT` or `STDERR` respectively.

The exception to this is when Terragrunt is running a process in "Headless Mode", where it will instead emit stdout and stderr terminal output to `INFO` and `ERROR` log levels respectively. This happens when

All other log levels are standard, and are used by Terragrunt to log its own messages.

For example:

```bash
$ terragrunt --terragrunt-log-level debug plan
14:20:38.431 DEBUG  Terragrunt Version: 0.0.0
14:20:38.431 DEBUG  Did not find any locals block: skipping evaluation.
14:20:38.431 DEBUG  Running command: tofu --version
14:20:38.431 DEBUG  Engine is not enabled, running command directly in .
14:20:38.451 DEBUG  tofu version: 1.8.5
14:20:38.451 DEBUG  Reading Terragrunt config file at ./terragrunt.hcl
14:20:38.451 DEBUG  Did not find any locals block: skipping evaluation.
14:20:38.451 DEBUG  Did not find any locals block: skipping evaluation.
14:20:38.452 DEBUG  Running command: tofu init
14:20:38.452 DEBUG  Engine is not enabled, running command directly in .
14:20:38.469 INFO   tofu: Initializing the backend...
14:20:38.470 INFO   tofu: Initializing provider plugins...
14:20:38.470 INFO   tofu: OpenTofu has been successfully initialized!
14:20:38.470 INFO   tofu:
14:20:38.470 INFO   tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
14:20:38.470 INFO   tofu: any changes that are required for your infrastructure. All OpenTofu commands
14:20:38.470 INFO   tofu: should now work.
14:20:38.470 INFO   tofu: If you ever set or change modules or backend configuration for OpenTofu,
14:20:38.470 INFO   tofu: rerun this command to reinitialize your working directory. If you forget, other
14:20:38.470 INFO   tofu: commands will detect it and remind you to do so if necessary.
14:20:38.470 DEBUG  Running command: tofu plan
14:20:38.470 DEBUG  Engine is not enabled, running command directly in .
14:20:38.490 STDOUT tofu: No changes. Your infrastructure matches the configuration.
14:20:38.490 STDOUT tofu: OpenTofu has compared your real infrastructure against your configuration and
14:20:38.490 STDOUT tofu: found no differences, so no changes are needed.
```

Here, we have three types of log messages:

1. `DEBUG` messages from Terragrunt itself. By default, Terragrunt's log level is `INFO`, but we've set it to `DEBUG` using the `--terragrunt-log-level` flag.
2. `STDOUT` messages from OpenTofu. These are messages that OpenTofu would normally print directly to the terminal, but instead, Terragrunt captures them and logs them as `STDOUT` log messages, along with timestamps and other metadata.
3. `INFO` messages from Terragrunt [auto-init](/docs/features/auto-init). These were initially emitted by OpenTofu, however the user did not specifically ask for them, so Terragrunt logs them as `INFO` messages.

## Enrichment

The reason Terragrunt enriches stdout/stderr from the processes is that it is often very useful to have this extra metadata.

For example:

```bash
$ terragrunt run-all plan
14:27:45.359 INFO   The stack at . will be processed in the following order for command plan:
Group 1
- Module ./unit-1
- Module ./unit-2


14:27:45.399 INFO   [unit-2] tofu: Initializing the backend...
14:27:45.399 INFO   [unit-1] tofu: Initializing the backend...
14:27:45.400 INFO   [unit-2] tofu: Initializing provider plugins...
14:27:45.400 INFO   [unit-2] tofu: OpenTofu has been successfully initialized!
14:27:45.400 INFO   [unit-2] tofu:
14:27:45.400 INFO   [unit-2] tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
14:27:45.400 INFO   [unit-2] tofu: any changes that are required for your infrastructure. All OpenTofu commands
14:27:45.400 INFO   [unit-2] tofu: should now work.
14:27:45.400 INFO   [unit-2] tofu: If you ever set or change modules or backend configuration for OpenTofu,
14:27:45.400 INFO   [unit-2] tofu: rerun this command to reinitialize your working directory. If you forget, other
14:27:45.400 INFO   [unit-2] tofu: commands will detect it and remind you to do so if necessary.
14:27:45.400 INFO   [unit-1] tofu: Initializing provider plugins...
14:27:45.400 INFO   [unit-1] tofu: OpenTofu has been successfully initialized!
14:27:45.400 INFO   [unit-1] tofu:
14:27:45.400 INFO   [unit-1] tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
14:27:45.400 INFO   [unit-1] tofu: any changes that are required for your infrastructure. All OpenTofu commands
14:27:45.400 INFO   [unit-1] tofu: should now work.
14:27:45.400 INFO   [unit-1] tofu: If you ever set or change modules or backend configuration for OpenTofu,
14:27:45.400 INFO   [unit-1] tofu: rerun this command to reinitialize your working directory. If you forget, other
14:27:45.400 INFO   [unit-1] tofu: commands will detect it and remind you to do so if necessary.
14:27:45.422 STDOUT [unit-2] tofu: No changes. Your infrastructure matches the configuration.
14:27:45.423 STDOUT [unit-2] tofu: OpenTofu has compared your real infrastructure against your configuration and
14:27:45.423 STDOUT [unit-2] tofu: found no differences, so no changes are needed.
14:27:45.423 STDOUT [unit-1] tofu: No changes. Your infrastructure matches the configuration.
14:27:45.423 STDOUT [unit-1] tofu: OpenTofu has compared your real infrastructure against your configuration and
14:27:45.423 STDOUT [unit-1] tofu: found no differences, so no changes are needed.
```

Here you see two different units being run by Terragrunt concurrently, and stdout/stderr for each being emitted in real time. This is really helpful when managing IaC at scale, as it lets you know exactly what each unit in your stack is doing, and how long it is taking.

It's easier to see the impact of this enrichment if we turn it off, so let's use the [bare](/docs/reference/log-formatting/#bare) preset described in [Log Formatting](/docs/reference/log-formatting).

```bash
$ terragrunt run-all --terragrunt-log-format bare plan
INFO[0000] The stack at /Users/yousif/tmp/testing-stdout-stderr-split will be processed in the following order for command plan:
Group 1
- Module /Users/yousif/tmp/testing-stdout-stderr-split/unit-1
- Module /Users/yousif/tmp/testing-stdout-stderr-split/unit-2



Initializing the backend...

Initializing provider plugins...

OpenTofu has been successfully initialized!

You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.

No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.

Initializing the backend...

Initializing provider plugins...

OpenTofu has been successfully initialized!

You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.

No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.
```

This tells Terragrunt to log messages from OpenTofu/Terraform without any enrichment. As you can see, it's not as easy to disambiguate the messages from the two units, so it helps to use Terragrunt's default log format when managing IaC at scale.

## Exceptions to enrichment

There are exceptions to the general rule that Terragrunt logs stdout/stderr from the processes it runs as `STDOUT` and `STDERR` respectively when not in Headless Mode.

Because Terragrunt is an IaC orchestrator, it uses its awareness of OpenTofu/Terraform usage to recognize certain circumstances when a user is likely to want stdout/stderr to be emitted exactly as it would when running a process directly.

An example of this is `terragrunt output`:

```bash
$ terragrunt output -json
15:20:07.759 INFO   tofu: Initializing the backend...
15:20:07.759 INFO   tofu: Initializing provider plugins...
15:20:07.759 INFO   tofu: OpenTofu has been successfully initialized!
15:20:07.759 INFO   tofu:
15:20:07.759 INFO   tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
15:20:07.759 INFO   tofu: any changes that are required for your infrastructure. All OpenTofu commands
15:20:07.759 INFO   tofu: should now work.
15:20:07.759 INFO   tofu: If you ever set or change modules or backend configuration for OpenTofu,
15:20:07.759 INFO   tofu: rerun this command to reinitialize your working directory. If you forget, other
15:20:07.759 INFO   tofu: commands will detect it and remind you to do so if necessary.
{
  "something": {
    "sensitive": false,
    "type": "string",
    "value": "Hello, World!"
  },
  "something_else": {
    "sensitive": false,
    "type": "string",
    "value": "Goodbye, World!"
  }
}
```

As you can see, the output from OpenTofu here isn't being enriched even though the user explicitly asked Terragrunt to run `output -json`.

This is because the user is pretty likely to want to programmatically interact with the output of `output`, and so Terragrunt doesn't enrich it.

If, for example, you wanted to use a tool like `jq` to parse the output of `terragrunt output -json`, you could do that without having to worry about Terragrunt's metadata getting in the way, or disabling anything with an extra flag.

```bash
$ terragrunt output -json | jq '.something'
15:24:40.310 INFO   tofu: Initializing the backend...
15:24:40.311 INFO   tofu: Initializing provider plugins...
15:24:40.311 INFO   tofu: OpenTofu has been successfully initialized!
15:24:40.311 INFO   tofu:
15:24:40.311 INFO   tofu: You may now begin working with OpenTofu. Try running "tofu plan" to see
15:24:40.311 INFO   tofu: any changes that are required for your infrastructure. All OpenTofu commands
15:24:40.311 INFO   tofu: should now work.
15:24:40.311 INFO   tofu: If you ever set or change modules or backend configuration for OpenTofu,
15:24:40.311 INFO   tofu: rerun this command to reinitialize your working directory. If you forget, other
15:24:40.311 INFO   tofu: commands will detect it and remind you to do so if necessary.
{
  "sensitive": false,
  "type": "string",
  "value": "Hello, World!"
}
```

## Streaming and buffering

While Terragrunt logs stdout from OpenTofu/Terraform in real time, it buffers each line of stdout before logging it. This is because Terragrunt needs to be able to buffer stdout to prevent different units from interleaving their log messages.

Depending on what you're doing with Terragrunt, this might occasionally result in issues when multiple units are running concurrently and they are each producing multi-line output that is more convenient to be read independently. In these cases, you can do some post-processing on the logs to read the units in isolation.

For example:

```bash
$ terragrunt run-all apply --terragrunt-no-color --terragrunt-non-interactive > logs
16:01:51.164 INFO   The stack at . will be processed in the following order for command apply:
Group 1
- Module ./unit1
- Module ./unit2

```

```bash
$ grep '\[unit1\]' < logs
16:01:51.272 STDOUT [unit1] tofu: null_resource.empty: Refreshing state... [id=3335573617542340690]
16:01:51.279 STDOUT [unit1] tofu: OpenTofu used the selected providers to generate the following execution
16:01:51.279 STDOUT [unit1] tofu: plan. Resource actions are indicated with the following symbols:
16:01:51.279 STDOUT [unit1] tofu: -/+ destroy and then create replacement
16:01:51.279 STDOUT [unit1] tofu: OpenTofu will perform the following actions:
16:01:51.279 STDOUT [unit1] tofu:   # null_resource.empty must be replaced
16:01:51.279 STDOUT [unit1] tofu: -/+ resource "null_resource" "empty" {
16:01:51.279 STDOUT [unit1] tofu:       ~ id       = "3335573617542340690" -> (known after apply)
16:01:51.279 STDOUT [unit1] tofu:       ~ triggers = { # forces replacement
16:01:51.280 STDOUT [unit1] tofu:           ~ "always_run" = "2025-01-09T21:01:17Z" -> (known after apply)
16:01:51.280 STDOUT [unit1] tofu:         }
16:01:51.280 STDOUT [unit1] tofu:     }
16:01:51.280 STDOUT [unit1] tofu: Plan: 1 to add, 0 to change, 1 to destroy.
16:01:51.280 STDOUT [unit1] tofu:
16:01:51.297 STDOUT [unit1] tofu: null_resource.empty: Destroying... [id=3335573617542340690]
16:01:51.297 STDOUT [unit1] tofu: null_resource.empty: Destruction complete after 0s
16:01:51.300 STDOUT [unit1] tofu: null_resource.empty: Creating...
16:01:51.301 STDOUT [unit1] tofu: null_resource.empty: Provisioning with 'local-exec'...
16:01:51.301 STDOUT [unit1] tofu: null_resource.empty (local-exec): Executing: ["/bin/sh" "-c" "echo 'sleeping...'; sleep 1; echo 'done sleeping'"]
16:01:51.304 STDOUT [unit1] tofu: null_resource.empty (local-exec): sleeping...
16:01:52.311 STDOUT [unit1] tofu: null_resource.empty (local-exec): done sleeping
16:01:52.312 STDOUT [unit1] tofu: null_resource.empty: Creation complete after 1s [id=4749136145104485309]
16:01:52.322 STDOUT [unit1] tofu:
16:01:52.322 STDOUT [unit1] tofu: Apply complete! Resources: 1 added, 0 changed, 1 destroyed.
16:01:52.322 STDOUT [unit1] tofu:
```

```bash
$ grep '\[unit2\]' < logs
16:01:51.273 STDOUT [unit2] tofu: null_resource.empty: Refreshing state... [id=7532622543468447677]
16:01:51.280 STDOUT [unit2] tofu: OpenTofu used the selected providers to generate the following execution
16:01:51.280 STDOUT [unit2] tofu: plan. Resource actions are indicated with the following symbols:
16:01:51.280 STDOUT [unit2] tofu: -/+ destroy and then create replacement
16:01:51.280 STDOUT [unit2] tofu: OpenTofu will perform the following actions:
16:01:51.280 STDOUT [unit2] tofu:   # null_resource.empty must be replaced
16:01:51.280 STDOUT [unit2] tofu: -/+ resource "null_resource" "empty" {
16:01:51.280 STDOUT [unit2] tofu:       ~ id       = "7532622543468447677" -> (known after apply)
16:01:51.280 STDOUT [unit2] tofu:       ~ triggers = { # forces replacement
16:01:51.280 STDOUT [unit2] tofu:           ~ "always_run" = "2025-01-09T21:01:17Z" -> (known after apply)
16:01:51.280 STDOUT [unit2] tofu:         }
16:01:51.280 STDOUT [unit2] tofu:     }
16:01:51.280 STDOUT [unit2] tofu: Plan: 1 to add, 0 to change, 1 to destroy.
16:01:51.280 STDOUT [unit2] tofu:
16:01:51.297 STDOUT [unit2] tofu: null_resource.empty: Destroying... [id=7532622543468447677]
16:01:51.297 STDOUT [unit2] tofu: null_resource.empty: Destruction complete after 0s
16:01:51.300 STDOUT [unit2] tofu: null_resource.empty: Creating...
16:01:51.301 STDOUT [unit2] tofu: null_resource.empty: Provisioning with 'local-exec'...
16:01:51.301 STDOUT [unit2] tofu: null_resource.empty (local-exec): Executing: ["/bin/sh" "-c" "echo 'sleeping...'; sleep 1; echo 'done sleeping'"]
16:01:51.303 STDOUT [unit2] tofu: null_resource.empty (local-exec): sleeping...
16:01:52.311 STDOUT [unit2] tofu: null_resource.empty (local-exec): done sleeping
16:01:52.312 STDOUT [unit2] tofu: null_resource.empty: Creation complete after 1s [id=6569505210291935319]
16:01:52.322 STDOUT [unit2] tofu:
16:01:52.322 STDOUT [unit2] tofu: Apply complete! Resources: 1 added, 0 changed, 1 destroyed.
16:01:52.322 STDOUT [unit2] tofu:
```

## Disabling logs

Finally, you can also disable logs entirely like so:

```bash
$ terragrunt --terragrunt-log-disable plan

Initializing the backend...

Initializing provider plugins...

OpenTofu has been successfully initialized!

You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.

No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.
```

This will give you the closes experience to using OpenTofu/Terraform directly, with Terragrunt doing all of its work in the background.
