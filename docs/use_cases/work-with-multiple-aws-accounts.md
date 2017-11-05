---
title: Work with multiple AWS accounts
layout: single
author_profile: true
sidebar:
  nav: "multiple-aws-accounts"
---

## Motivation

The most secure way to manage infrastructure in AWS is to use [multiple AWS 
accounts](https://aws.amazon.com/answers/account-management/aws-multi-account-security-strategy/). You define all your 
IAM users in one account (e.g., the "security" account) and deploy all of your infrastructure into a number of other
accounts (e.g., the "dev", "stage", and "prod" accounts). To access those accounts, you login to the security account
and [assume an IAM role](http://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html) in the other accounts.

There are a few ways to assume IAM roles when using AWS CLI tools, such as Terraform:  

1. One option is to create a named [profile](http://docs.aws.amazon.com/cli/latest/userguide/cli-multiple-profiles.html),
   each with a different [role_arn](http://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html) parameter. You then
   tell Terraform which profile to use via the `AWS_PROFILE` environment variable. The downside to using profiles is 
   that you have to store your AWS credentials in plaintext on your hard drive.  

1. Another option is to use environment variables and the [AWS CLI](https://aws.amazon.com/cli/). You first set the
   credentials for the security account (the one where your IAM users are defined) as the environment variables
   `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` and run `aws sts assume-role --role-arn <ROLE>`. This gives you 
   back a blob of JSON that contains new `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` values you can set as 
   environment variables to allow Terraform to use that role. The advantage of this approach is that you can store your
   AWS credentials in a secret store and never write them to disk in plaintext. The disadvantage is that assuming an
   IAM role requires several tedious steps. Worse yet, the credentials you get back from the `assume-role` command are
   only good for up to 1 hour, so you have to repeat this process often.

1. A final option is to modify your AWS provider with the [assume_role 
   configuration](https://www.terraform.io/docs/providers/aws/#assume-role) and your S3 backend with the [role_arn
   parameter](https://www.terraform.io/docs/backends/types/s3.html#role_arn). You can then set the credentials for the 
   security account (the one where your IAM users are defined) as the environment variables `AWS_ACCESS_KEY_ID` and 
   `AWS_SECRET_ACCESS_KEY` and when you run `terraform apply` or `terragrunt apply`, Terraform/Terragrunt will assume
   the IAM role you specify automatically. The advantage of this approach is that you can store your
   AWS credentials in a secret store and never write them to disk in plaintext, and you get fresh credentials on every
   run of `apply`, without the complexity of calling `assume-role`. The disadvantage is that you have to modify all
   your Terraform / Terragrunt code to set the `role_arn` param and your Terraform backend configuration will change
   (and prompt you to manually confirm the update!) every time you change the IAM role you're using.

To avoid these frustrating trade-offs, you can configure Terragrunt to assume an IAM role for you, as described next.


## Configuring Terragrunt to assume an IAM role

To tell Terragrunt to assume an IAM role, just set the `--terragrunt-iam-role` command line argument:

```bash
terragrunt --terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME" apply
```

Alternatively, you can set the `TERRAGRUNT_IAM_ROLE` environment variable:

```bash
export TERRAGRUNT_IAM_ROLE="arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
terragrunt apply
```

Terragrunt will call the `sts assume-role` API on your behalf and expose the credentials it gets back as environment 
variables when running Terraform. The advantage of this approach is that you can store your AWS credentials in a secret 
store and never write them to disk in plaintext, you get fresh credentials on every run of Terragrunt, without the 
complexity of calling `assume-role` yourself, and you don't have to modify your Terraform code or backend configuration
at all.
