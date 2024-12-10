---
layout: collection-browser-doc
title: Contributing
category: community
excerpt: >-
  Terragrunt is an open source project, and contributions from the community are very welcome!
tags: ["contributing", "community"]
order: 300
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt is an open source project, and contributions from the community are very welcome\! Please check out the
[Contribution Guidelines](#contribution-guidelines) and [Developing Terragrunt](#developing-terragrunt) for
instructions.

## Contribution Guidelines

Contributions to this repo are very welcome! We follow a fairly standard [pull request
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

### Running tests

**Note**: The tests in the `dynamodb` folder for Terragrunt run against a real AWS account and will add and remove real data from DynamoDB. DO NOT hit `CTRL+C` while the tests are running, as this will prevent them from cleaning up temporary tables and data in DynamoDB. We are not responsible for any charges you may incur.

Before running the tests, you must configure your [AWS credentials]({{site.baseurl}}/docs/features/aws-auth/#aws-credentials) and [AWS IAM policies]({{site.baseurl}}/docs/features/aws-auth/#aws-iam-policies).

To run all the tests:

```bash
go test -v ./...
```

To run only the tests in a specific package, such as the package `remote`:

```bash
cd remote
go test -v
```

And to run a specific test, such as `TestToTerraformRemoteConfigArgsNoBackendConfigs` in package `remote`:

```bash
cd remote
go test -v -run TestToTerraformRemoteConfigArgsNoBackendConfigs
```

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

Every source file in this project should be formatted with `go fmt`. There are few helper scripts and targets in the Makefile that can help with this (mostly taken from the [terraform repo](https://github.com/hashicorp/terraform/)):

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
