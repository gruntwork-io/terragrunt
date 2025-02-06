---
title: Contributing
description: Contributing to Terragrunt
slug: docs/community/contributing
---

## Contribution Guidelines

Contributions to Terragrunt are very welcome! We follow a fairly standard [pull request
process](https://help.github.com/articles/about-pull-requests/) for contributions, subject to the following guidelines:

- [Contribution Guidelines](#contribution-guidelines)
  - [File a GitHub issue or write an RFC](#file-a-github-issue-or-write-an-rfc)
  - [Update the documentation](#update-the-documentation)
  - [Update the tests](#update-the-tests)
  - [Update the code](#update-the-code)
  - [Create a pull request](#create-a-pull-request)
  - [Merge and release](#merge-and-release)
- [Developing Terragrunt](#developing-terragrunt)
  - [Running locally](#running-locally)
  - [Dependencies](#dependencies)
  - [Linting](#linting)
  - [Running tests](#running-tests)
  - [Debug logging](#debug-logging)
  - [Error handling](#error-handling)
  - [Formatting](#formatting)
- [Terragrunt Releases](#terragrunt-releases)
  - [When to Cut a New Release](#when-to-cut-a-new-release)
  - [How to Create a New Release](#how-to-create-a-new-release)
  - [Pre-releases](#pre-releases)

### File a GitHub issue or write an RFC

Before starting any work, we recommend filing a GitHub issue in this repo. This is your chance to ask questions and
get feedback from the maintainers and the community before you sink a lot of time into writing (possibly the wrong)
code. If there is anything you're unsure about, just ask!

Sometimes, the scope of the feature proposal is large enough that it requires major updates to the code base to
implement. In these situations, a maintainer may suggest writing up an RFC that describes the feature in more details
than what can be reasonably captured in an enhancement.

To write an RFC, click the [RFC](https://github.com/gruntwork-io/terragrunt/issues/new?assignees=&labels=rfc%2Cpending-decision&projects=&template=02-rfc.yml) button in the issues tab.

This will present you a template you can fill out to describe the feature you want to propose.

### Update the documentation

We recommend updating the documentation _before_ updating any code (see [Readme Driven
Development](http://tom.preston-werner.com/2010/08/23/readme-driven-development.html)). This ensures the documentation
stays up to date and allows you to think through the problem at a high level before you get lost in the weeds of
coding.

The documentation is built with Jekyll and hosted on the Github Pages from `docs` folder on `main` branch. Check out
[Terragrunt website](https://github.com/gruntwork-io/terragrunt/tree/main/docs#working-with-the-documentation) to
learn more about working with the documentation.

### Update the tests

We also recommend updating the automated tests _before_ updating any code (see [Test Driven
Development](https://en.wikipedia.org/wiki/Test-driven_development)). That means you add or update a test case,
verify that it's failing with a clear error message, and _then_ make the code changes to get that test to pass. This
ensures the tests stay up to date and verify all the functionality in this Module, including whatever new
functionality you're adding in your contribution. Check out [Developing Terragrunt](#developing-terragrunt)
for instructions on running the automated tests.

### Update the code

At this point, make your code changes and use your new test case to verify that everything is working. Check out
[Developing Terragrunt](#developing-terragrunt) for instructions on how to build and run Terragrunt locally.

We have a [style guide](https://gruntwork.io/guides/style%20guides/golang-style-guide/) for the Go programming language,
in which we documented some best practices for writing Go code. Please ensure your code adheres to the guidelines
outlined in the guide.

### Create a pull request

[Create a pull request](https://help.github.com/articles/creating-a-pull-request/) with your changes. Please make sure
to include the following:

1. A description of the change, including a link to your GitHub issue.
1. The output of your automated test run, preferably in a [GitHub Gist](https://gist.github.com/).
   We cannot run automated tests for pull requests automatically due to
   [security concerns](https://circleci.com/docs/2.0/oss/#pass-secrets-to-builds-from-forked-pull-requests),
   so we need you to manually provide this test output so we can verify that everything is working.
1. Any notes on backwards incompatibility.

### Merge and release

The maintainers for this repo will review your code and provide feedback. If everything looks good, they will merge the
code and release a new version, which you'll be able to find in the [releases page](https://github.com/gruntwork-io/terragrunt/releases).

## Developing Terragrunt

### Running locally

To run Terragrunt locally, use the `go run` command:

```bash
go run main.go plan
```

### Dependencies

Terragrunt uses go modules (read more about the modules system in [the official
wiki](https://github.com/golang/go/wiki/Modules)). This means that dependencies are automatically installed when you use
any go command that compiles the code (`build`, `run`, `test`, etc.).

### Linting

Terragrunt uses [golangci-lint](https://golangci-lint.run/) to lint the golang code in the codebase. This is a helpful form of static analysis that can catch common bugs and issues related to performance, style and maintainability.

We use the linter as a guide to learn about how we can improve the Terragrunt codebase. We do not enforce 100% compliance with the linter. If you believe that an error thrown by the linter is irrelevant, use the documentation on [false-positives](https://golangci-lint.run/usage/false-positives/) to suppress that error, along with an explanation of why you believe the error is a false positive.

If you feel like the linter is missing a check that would be useful for improving the code quality of Terragrunt, please open an issue to discuss it, then open a pull request to add the check.

There are two lint configurations currently in use:

- **Default linter**

  This is the default configuration that is used when running `golangci-lint run`. The configuration for this lint is defined in the `.golangci.yml` file.

  These lints **must** pass before any code is merged into the `main` branch.

- **Strict linter**

  This is the more strict configuration that is used to check for additional issues in pull requests. This configuration is defined in the `.strict.golanci.yml` file.

  These lints **do not have to pass** before code is merged into the `main` branch, but the results are useful to look at to improve code quality.

#### Default linter

Before any tests run in our continuous integration suite, they must pass the default linter. This is to ensure an acceptable floor for code quality in the codebase.

To run the default linter directly, use:

```bash
golangci-lint run
```

There's also a Makefile recipe that runs the default linter:

```bash
make run-lint
```

If possible, you are advised to [integrate the linter into your code editor](https://golangci-lint.run/welcome/integrations/) to get immediate feedback as edit Terragrunt code.

#### Strict linter

To run the strict linter, use:

```bash
golangci-lint run -c .strict.golangci.yml
```

It's generally not practical to run the strict linter on the entire codebase, as it's very strict and will likely produce a lot of errors. Instead, you can run it on the files that have changed with respect to the `main` branch.

You can do that like so:

```bash
golangci-lint run -c .strict.golangci.yml --new-from-rev origin/main ./...
```

This is basically what the `run-strict-lint` Makefile recipe does:

```bash
make run-strict-lint
```

In our continuous integration suite, we run the strict linter on the files that have changed with respect to the `main` branch, and very intentionally do not enforce that the lints pass. We pay more attention to the results when the lint fails for a pull request that are changing few small files, as those are much more likely to pass. In those cases, maintainers will review the results, and may suggest changes that are necessary to improve code quality if they believe the cost of implementing the changes is low.

#### Markdownlint

In addition to the golang linter, we also use [markdownlint](https://github.com/DavidAnson/markdownlint) to lint the markdown files in the codebase. This is to ensure that the documentation is consistent and easy to read.

You'll want to check that the markdown files are linted correctly before submitting a pull request to update the docs. You can do this by running:

```bash
markdownlint \
    --disable MD013 MD024 \
    -- \
    docs
```

### Running tests

There are multiple different kinds of tests in the Terragrunt codebase, and each serves a different purpose.

#### Unit tests

These are tests that test individual functions in the codebase. They are located in the same package as the code they are testing and are suffixed `*_test.go`.

They use a package directive that is suffixed `_test` of the package they test to force them to only test exported functions of that package, while residing in the same directory.

The idea behind this practice is to keep the tests close to the code they are testing, and to force them to only test the public API of the package. This allows implementation details of particular functions to change without breaking tests, as long as the public API behaves the same.

In general, if you are editing Terragrunt code, and there isn't a unit test that covers the code you are updating, it's probably a good idea to add one. If there is a unit test for the code you are updating, you should make sure that you run that test after any update to ensure that you haven't broken anything.

When possible, introduce new tests for code _before_ you start making changes. This is a practice known as [Test Driven Development](https://en.wikipedia.org/wiki/Test-driven_development).

You can run the unit tests for a particular package by running:

```bash
go test ./path/to/package
```

To specifically run a single test, you can use the `-run` flag:

```bash
go test -run TestFunctionName ./path/to/package
```

There are many ways to customize the `go test` command, including using flags like `-v` to get more verbose output. To learn more about go testing, read the [official documentation](https://pkg.go.dev/testing).

#### Integration tests

These are tests that test integrations between multiple parts of the Terragrunt codebase, and external services. They generally invoke Terragrunt as if you were using it from the command line.

These tests are located in the `test` directory, and are suffixed `*_test.go`.

Often, these tests run against test fixtures, which are small Terragrunt configurations that emulate specific real-world scenarios. These test fixtures are located in the `test/fixtures` directory.

To run the integration tests, you can use the `go test` command:

```bash
go test ./test
```

Note that integration tests can be slow, as they often involve running full Terragrunt commands, and that frequently involves spawning new processes. As a result, you may want to run only a subset of the tests while developing. You can do this by using the `-run` flag:

```bash
go test -run 'TestBeginningOfFunctionName*' ./test
```

This will run all tests that start with `TestBeginningOfFunctionName`.

Note that some tests may require that you opt-in for them to be tested. This is because they may require access to external services that you need to authenticate with or use a specific external tool that you might not have installed. In these cases, we use [golang build tags](https://pkg.go.dev/go/build) to conditionally compile the tests. You can run these tests by setting the appropriate build tag before testing.

For example, AWS tests are tagged using the `aws` build tag. To run these tests, you can use the `-tags` flag set in the `GOFLAGS` environment variable like so:

```bash
GOFLAGS='-tags=aws' go test -run 'TestAwsInitHookNoSourceWithBackend' .
```

Depending on how you've configured your editor, you may need to make sure that your editor has the `GOFLAGS` environment variable set before starting for the best development experience:

```bash
export GOFLAGS='-tags=aws'
neovim .
```

In general, we try to make sure that any test that requires a build tag is also consistently prefixed a certain way so that they can be tested independently.

For example, all AWS tests are prefixed with `TestAws*`.

#### Race tests

Given that Terragrunt is a tool that frequently involves concurrently running multiple things at once, there's always a risk for race conditions to occur. As such, there are dedicated tests that are run with the `-race` flag in CI to use golang's built-in tooling for identifying race conditions.

In general, when encountering a bug caused by a race condition in the wild, we endeavor to write a test for it, and add it to the `./test/race_test.go` file to avoid regressions in the future. If you want to make sure that new code you are writing doesn't introduce a race condition, add a test for it in the `race_test.go` file.

We can do a better job of finding candidates for additional testing here, so if you are interested in helping out, please open an issue to discuss it.

#### Benchmark tests

Benchmark tests are tests that are run with the `-bench` flag to the `go test` command. They are used to measure the performance of a particular function or set of functions.

You can find them by looking for tests that start with `Benchmark*` instead of `Test*` in the codebase.

In general, we have inadequate benchmark testing in the Terragrunt codebase, and want to improve this. If you are interested in helping out, please open an issue to discuss it.

Prior to the release of Terragrunt 1.0, we will have a concerted effort to improve the benchmark testing in the codebase.

#### Continuous Integration

Currently, all of the testing mentioned above is run automatically as part of our continuous integration suite in CircleCI.

This has caused some issues in the past, as many contributors don't have CircleCI accounts, and it can be frustrating to be unable to replicate the automated tests that we run for contributors when they submit a pull request.

We acknowledge this issue, and are working on a solution to make it easier for contributors to run the tests for this project themselves. This will involve moving the tests to GitHub Actions, which will allow contributors to run the tests on their own fork of the project, without needing to sign up for a new account.

This work will be completed prior to the release of Terragrunt 1.0.

### Debug logging

If you set the `TERRAGRUNT_DEBUG` environment variable to "true", the stack trace for any error will be printed to stdout when you run the app.

Additionally, newer features introduced in v0.19.0 (such as `locals` and `dependency` blocks) can output more verbose logging if you set the `TG_LOG` environment variable to `debug`.

### Error handling

In this project, we try to ensure that:

1. Every error has a stacktrace. This makes debugging easier.

2. Every error generated by our own code (as opposed to errors from Go built-in functions or errors from 3rd party libraries) has a custom type. This makes error handling more precise, as we can decide to handle different types of errors differently.

To accomplish these two goals, we have created an `errors` package that has several helper methods, such as `errors.New(err error)`, which wraps the given `error` in an Error object that contains a stacktrace. Under the hood, the `errors` package is using the [go-errors](https://github.com/go-errors/errors) library, but this may change in the future, so the rest of the code should not depend on `go-errors` directly.

Here is how the `errors` package should be used:

1. Any time you want to create your own error, create a custom type for it, and when instantiating that type, wrap it with a call to `errors.New`. That way, any time you call a method defined in the Terragrunt code, you know the error it returns already has a stacktrace and you don’t have to wrap it yourself.

2. Any time you get back an error object from a function built into Go or a 3rd party library, immediately wrap it with `errors.New`. This gives us a stacktrace as close to the source as possible.

3. If you need to get back the underlying error, you can use the `errors.IsError` and `errors.Unwrap` functions.

### Formatting

Every source file in this project should be formatted with `go fmt`. There are few helper scripts and targets in the Makefile that can help with this (mostly taken from the [terraform repo](https://github.com/hashicorp/terraform/) when it was MPL licensed):

1. `make fmtcheck` Checks to see if all source files are formatted. Exits 1 if there are unformatted files.

2. `make fmt` Formats all source files with `gofmt`.

3. `make install-pre-commit-hook`

    Installs a git pre-commit hook that will run all of the source files through `gofmt`.

To ensure that your changes get properly formatted, please install the git pre-commit hook with `make install-pre-commit-hook`.

## Terragrunt Releases

Terragrunt releases follow [semantic versioning guidelines (semver)](https://semver.org/).

Note that as of 2024/10/17, Terragrunt is still pre-1.0, so breaking changes may still be introduced in minor releases. We will try to minimize these changes as much as possible, but they may still happen.

Once 1.0 is released, Terragrunt backwards compatibility will be guaranteed for all minor releases.

This documentation should be updated at that time to reflect the new policy. If it has not, please file a bug report.

### When to Cut a New Release

While Terragrunt is still pre-1.0, maintainers will cut a new release whenever a new feature is added or a bug is fixed. Maintainers will exercise their best judgment to determine when a new release is necessary, and bias towards cutting a new release as frequently as possible when in doubt.

Post-1.0, maintainers will slow down the release cadence using a different release cadence. This documentation will be updated at that time to reflect the new policy.

### How to Create a New Release

To release a new version of Terragrunt, go to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases) and cut a new release off the `main` branch. Ensure that the new release uses the **Set as a pre-release** checkbox initially.

The CircleCI job for this repo has been configured to:

1. Automatically detect new tags.

2. Build binaries for every OS using that tag as a version number.

3. Upload the binaries to the release in GitHub.

See `.circleci/config.yml` for details.

Follow the CircleCI job to ensure that the binaries are uploaded correctly. Once the job is successful, go back to the release, uncheck the **Set as a pre-release** checkbox and check the **Set as the latest release** checkbox.

### Pre-releases

Occasionally, Terragrunt maintainers will cut a pre-release to get feedback on the UI/UX for a new feature or to test it in the wild before making it generally available.

These releases are generally cut off a feature branch, in order to keep the `main` branch stable and releasable at all times.

Pre-releases are tagged with a pre-release suffix that looks like the following: `-alpha2024101701`, `-beta2024101701`, etc. with the following information:

- Channel: e.g. `alpha`, `beta` (indicating the stability of the release)

  The `alpha` and `beta` channels have the following meaning in Terragrunt:

  - `alpha`: This release is not recommended for external use. It is intended for early adoption by Gruntwork developers testing new features.
  - `beta`: This release is recommended for testing in non-production environments only. It is intended for testing out new features with stakeholders external to Gruntwork before a general release.

- Date: e.g. `20241017` (indicating the date the release was cut without dashes or slashes)
- Incremental number: e.g. `01` (indicating the number of pre-releases cut on that day)

This suffix is appended to the end of the next appropriate version number, e.g. if the current release is `v0.19.1`, and the next appropriate version number is `v0.20.0` based on semver, the pre-release tag would be `v0.20.0-alpha2024101701`.
