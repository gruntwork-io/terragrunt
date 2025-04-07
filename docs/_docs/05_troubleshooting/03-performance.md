---
layout: collection-browser-doc
title: Performance
category: troubleshooting
categories_url: troubleshooting
excerpt: Learn how to improve the performance of Terragrunt.
tags: ["Performance", "Performance Tuning", "Performance Optimization"]
order: 503
nav_title: Documentation
nav_title_link: /docs/
slug: performance
---

## Easy Wins

Normally, it's best practice to start by measuring performance before making any changes. This allows you to understand the impact of your changes, and to identify areas for improvement.

However, given the nature of the problems that Terragrunt solves, there are some obvious wins that you can make without measuring performance, if you're aware of the tradeoffs.

### Provider Cache

One of the most expensive things that OpenTofu/Terraform does, from a bandwidth and disk utilization perspective, is download and install providers. These are large binary files that are downloaded from the internet, and not cached across units by default.

You can significantly reduce the amount of time taken by Terragrunt runs by enabling the [provider cache server](/docs/features/provider-cache-server/), like this:

```bash
terragrunt run-all plan --provider-cache
```

#### Provider Cache - Gotchas

The provider cache server is a single server that is used by all Terragrunt runs being performed in a given Terragrunt invocation. You will see the most benefit if you are using it in a command that will perform multiple OpenTofu/Terraform operations, like with the `--all` flag and the `--graph` flag.

When performing individual runs, like `terragrunt plan`, the provider cache server can be a net negative to performance, because starting and stopping the server might add more overhead than just downloading the providers. Whether this is the case depends on many factors, including network speed, the number of providers being downloaded, and whether or not the providers are already cached in the Terragrunt provider cache.

When in doubt, [measure the performance](#measuring-performance) before and after enabling the provider cache server to see if it's a net win for your use case.

We are coordinating with the OpenTofu team to improve the behavior of concurrent access to provider cache, so that this flag will be unnecessary for most users in the future.

See [#1939](https://github.com/opentofu/opentofu/issues/1483) for more details.

### Fetching Output From State

Under the hood, Terragrunt [dependency](/docs/reference/config-blocks-and-attributes/#dependency) blocks leverage the OpenTofu/Terraform `output -json` command to fetch outputs from one unit and leverage them in another.

The OpenTofu/Terraform `output -json` command does a bit more work than simply fetching output values from state, and a significant portion of that slowdown is loading providers, which it doesn't really need in most cases.

You can significantly improve the performance of dependency blocks by using the [--dependency-fetch-output-from-state](/docs/reference/cli-options/#dependency-fetch-output-from-state) flag. When the flag is set, Terragrunt will directly fetch the backend state file from S3 and parse it directly, avoiding any overhead incurred by calling the `output -json` command.

For example:

```bash
terragrunt run-all plan --dependency-fetch-output-from-state
```

#### Fetching Output From State - Gotchas

The first thing you need to be aware of when considering usage of the `--dependency-fetch-output-from-state` flag is that it only works for S3 backends. If you are using a different backend, this flag won't do anything.

Next, you should be aware that there is no guarantee that OpenTofu/Terraform will maintain the existing schema of their state files, so there is also no guarantee that the flag will work as expected in future versions of OpenTofu/Terraform.

We are coordinating with the OpenTofu team to improve the performance of the `output` command, and we hope that this flag will be unnecessary for most users in the future.

See [#1549](https://github.com/opentofu/opentofu/issues/1549) for more details.

### Skip Dependency Inputs

Terragrunt dependency blocks allow reading inputs directly from other dependencies. However, the mechanism required to support this capability introduces performance overhead during Terragrunt operations. Due to this performance impact, using this feature is heavily discouraged.

The reason for this is that Terragrunt needs to recursively read inputs from dependencies, and their dependencies, all the way down to the ancestral Terragrunt unit they all have in common to be able to expose the inputs to the dependent unit from its direct dependency (as the inputs from the direct dependency might be derived from a dependency of the direct dependency).

You can mitigate this performance penalty by using the [skip-dependency-inputs](/docs/reference/strict-mode/#skip-dependency-inputs) strict control.

Enabling this strict control will activate a breaking change in Terragrunt behavior, making it so that dependency blocks no longer parse dependencies of dependencies, which is required to read inputs from direct dependencies.

In the future, we will be removing this capability from Terragrunt, so you won't need to use this strict control.

### Skip Dependency Inputs - Gotchas

In general, you should use this strict control. The only reason not to use it is if you are using dependency blocks to read inputs from dependencies, which you should stop doing as soon as possible.

## Measuring Performance

Before diving into any particular performance optimization, it's important to first measure performance, and to make sure that you measure performance after any changes so that you understand the impact of your changes.

To measure performance, you can use multiple tools, depending on your role.

### End User

As an end user, you're advised to use the following tools to get a better understanding of the performance of Terragrunt.

#### OpenTelemetry

Use [OpenTelemetry](./02-open-telemetry.md) to collect traces from Terragrunt runs so that you can analyze the performance of individual operations when using Terragrunt.

This can be useful both to identify bottlenecks in Terragrunt, and to understand when performance changes can be attributed to integrations with other tools, like OpenTofu or Terraform.

#### Benchmark Usage

Use benchmarking tools like [Hyperfine](https://github.com/sharkdp/hyperfine) to run benchmarks of your Terragrunt usage to compare the performance of different versions of Terragrunt, or with different configurations.

You can use configurations like the `--warmup` flag to do some warmup runs before the actual benchmarking. This is useful to get a more accurate measurement of the performance of Terragrunt with cache populated, etc.

Here's an example of how to use Hyperfine to benchmark the performance of Terragrunt with two different configurations:

```bash
hyperfine -w 3 -r 5 'terragrunt run-all plan' 'terragrunt run-all plan --dependency-fetch-output-from-state'
```

### Terragrunt Developer

As a Terragrunt developer, you're advised to use the following tools to improve the performance of Terragrunt when improving the codebase.

#### Benchmark Tests

Use [Benchmark tests](/docs/community/contributing/#benchmark-tests) to measure the performance of particular subroutines in Terragrunt.

These benchmarks give you a good indication of the performance of a particular part of Terragrunt, and can help you identify areas for improvement. You can run benchmark tests like this:

```bash
go test -bench=BenchmarkSomeFunction
```

You can also run benchmarks with different configurations, like the following for getting memory allocation information as well:

```bash
go test -bench=BenchmarkSomeFunction -benchmem
```

You can learn more about benchmarking in Go by reading the [official documentation](https://pkg.go.dev/testing#hdr-Benchmarks).

#### Profiling

Use profiling tools like [pprof](https://github.com/google/pprof) to get a more detailed view of the performance of Terragrunt.

For example, you could use the following command to profile a particular test:

```bash
go test -run 'SomeTest' -cpuprofile=cpu.prof -memprofile=mem.prof
```

You can then use the `go tool pprof` command to analyze the profile data:

```bash
go tool pprof cpu.prof
```

It can be helpful to use the web interface to view the profile data using flame graphs, etc.

```bash
go tool pprof -http=:8080 cpu.prof
```

You can learn more about profiling in Go by reading the [official documentation](https://pkg.go.dev/cmd/pprof).
