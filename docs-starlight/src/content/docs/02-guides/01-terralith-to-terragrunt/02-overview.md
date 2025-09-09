---
title: Overview
description: Overview of the Terralith to Terragrunt guide
slug: docs/guides/terralith-to-terragrunt/overview
sidebar:
  order: 2
---

To demonstrate the journey from a Terralith to a scalable Terragrunt setup, we will build and deploy a complete, real-world application early on in this guide, then spend the rest of the guide refactoring the IaC that manages the infrastructure hosting it.

The architecture for our sample project is a simple serverless web application hosted in AWS, which consists of three main components:

1. A [Lambda](https://aws.amazon.com/lambda/)-backed website.
2. An [S3 bucket](https://aws.amazon.com/s3/) to store static assets.
3. A [DynamoDB table](https://aws.amazon.com/dynamodb/) to store metadata on those assets.

This application will allow users to view and vote on their favorite AI-generated images of cats. We've intentionally chosen these AWS serverless offerings as they are cost-effective and should be very cheap, if not free, for anyone following along with a new AWS account.

### What You'll Need

To provision the application we build as part of this guide, you will need an AWS account, and permissions to provision resources within it. If you don't have one, you can follow the official [instructions to sign up](https://signin.aws.amazon.com/signup?request_type=register) for one for free.

To manage the development dependencies for this project, this guide uses [mise](https://mise.jdx.dev/), a tool that helps manage project-specific runtimes and tools. You are welcome to install the required tools manually, but using `mise` (or another tool manager) is recommended when working with IaC, as reproducibility is paramount for ensuring that you can work effectively with colleagues (including future you) on shared infrastructure.

If you are happy to install development dependencies with `mise`, you can install it using the official [Mise](https://mise.jdx.dev/getting-started.html) installation guide.

If you would like to manually install all the development dependencies for this guide, you can install them here:

- [Terragrunt](https://terragrunt.gruntwork.io/docs/getting-started/install/)
- [OpenTofu](https://opentofu.org/docs/intro/install/)
- [NodeJS](https://nodejs.org/en/download)
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)

We will build this project from the ground up, but if you get lost at any point or want to skip ahead, you can find the completed project (as of each step) [here](https://github.com/gruntwork-io/terragrunt/tree/main/docs-starlight/src/fixtures/terralith-to-terragrunt).

Note that the content shown in code fences in this project will always be displayed in totality, so you can either copy them directly into the filename that's labeled at the top of the code fence for a file, or run the command directly in your terminal for commands. If a command starts with a `$`, the intent of the code fence is to demonstrate expected output, so you aren't expected to copy and paste it directly into your terminal.
