---
layout: collection-browser-doc
title: AWS Authentication
category: features
categories_url: features
excerpt: Learn how the Terragrunt handles AWS authentication.
tags: ["AWS", "Use cases", "CLI", "AWS IAM"]
order: 225
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/aws-auth/
    - /docs/features/work-with-multiple-aws-accounts/
---

## Motivation

AWS is by far the most popular OpenTofu/Terraform provider, and most Terragrunt users are using it to manage AWS infrastructure, at least in part. As a consequence, Terragrunt has a number of features that make it easier to work with AWS, especially when you have to manage multiple AWS accounts.

The most secure way to manage AWS infrastructure is to segment infrastructure between [multiple AWS accounts](https://aws.amazon.com/organizations/getting-started/best-practices). Segmenting infrastructure in this way can ensure that developers are not granted permissions they don't need on infrastructure they don't manage. It's also a best practice from a safety perspective, as it helps to prevent accidental changes to sensitive resources like production infrastructure.

When working with multiple AWS accounts, a best practice is to temporarily assume roles within those AWS accounts to perform actions using mechanisms like [IAM Identity Center](https://aws.amazon.com/iam/identity-center/) or [OIDC](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_oidc.html). When using these technologies, users don't need any static users or credentials. All access is temporary, and permissions are determined by the role they assume.

These technologies allow you to securely assume least privilege access to a target AWS account, and perform actions that can only impact that AWS account, limiting blast radius.

There are a few ways to assume IAM roles when using AWS CLI tools, such as OpenTofu/Terraform:

1. One option is to create a named [profile](http://docs.aws.amazon.com/cli/latest/userguide/cli-multiple-profiles.html), each with a different [role_arn](http://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html) parameter. You then tell OpenTofu/Terraform which profile to use via the `AWS_PROFILE` environment variable.

   The downside to using profiles is that they can vary between users. One user might have a profile named `dev` that assumes a role in the `dev` account, while another user might have a profile named `development` that assumes the same role. This can lead to confusion and errors when sharing code between users. It also results in a requirement that all users have profiles set up on their local machines.

   Finally, this also presents a problem in CI/CD pipelines, where you typically don't want to store AWS credentials in plaintext on disk in order to have your CI/CD runner assume a role via a profile.

2. Another option is to use the [AWS CLI](https://aws.amazon.com/cli/). As a standard operating procedure, users are required to assume a role _before_ invoking OpenTofu/Terraform by running something like `aws sts assume-role --role-arn <ROLE>`, use the output of that command to set the appropriate environment variables, and the tool is run with those temporary credentials stored as environment variables.

   The downside to this approach is that it requires that users know this process and remember to do it correctly every time they want to use OpenTofu/Terraform. It's also a tedious process, and requires perrforming several steps to do it right.

   Worse yet, it requires that users repeat this process often, as the credentials you get back from the `assume-role` command expire. This is especially problematic if the OpenTofu/Terraform run is expected to take longer than the role assumption duration, and can expire mid-run.

3. A final option is to modify your AWS provider with the [assume_role configuration](https://www.terraform.io/docs/providers/aws/#assume-role) and your S3 backend with the [role_arn parameter](https://opentofu.org/docs/language/settings/backends/s3/#assume-role-configuration).

   The downside to managing your role assumption with the AWS provider is that all runs have to be performed with the same IAM role. This can be problematic if you have different users that assume different roles, depending on their need for elevated access, and as a best practice, the role assumed by CI/CD pipelines should be different from the role assumed by developers.

   The _way_ in which these roles are assumed also differ, as developers might use a web-based SSO portal to acquire temporary credentials, while CI/CD pipelines might use OIDC and assume a role using a web identity token.

To avoid these frustrating trade-offs, you can configure Terragrunt to assume an IAM role for you.

## Configuring Terragrunt to assume an IAM role

To tell Terragrunt to assume an IAM role, just set the [`--terragrunt-iam-role`](/docs/reference/cli-options/#terragrunt-iam-role) command line argument:

```bash
terragrunt apply --terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
```

Alternatively, you can set the `TERRAGRUNT_IAM_ROLE` environment variable:

```bash
export TERRAGRUNT_IAM_ROLE="arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
terragrunt apply
```

Additionally, you can specify an `iam_role` property in the terragrunt config:

```hcl
iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
```

Terragrunt will resolve the value of the option by first looking for the cli argument, then looking for the environment variable, then defaulting to the value specified in the config file.

Terragrunt will call the `sts assume-role` API on your behalf and expose the credentials it gets back as environment variables when running OpenTofu/Terraform. The advantage of this approach is that you can store your AWS credentials in a secret store and never write them to disk in plaintext, you get fresh credentials on every run of Terragrunt, without the complexity of calling `assume-role` yourself, and you don’t have to modify your OpenTofu/Terraform code or backend configuration at all.

## Leveraging OIDC role assumption

In addition, you can combine the `--terragrunt-iam-role` flag with the [`--terragrunt-iam-web-identity-token`](/docs/reference/cli-options/#terragrunt-iam-web-identity-token) to use the `AssumeRoleWithWebIdentity` API instead of the `AssumeRole` API.

This is especially convenient in the context of CI/CD pipelines, as it's generally a best practice to assume roles there via OIDC.

Configuring OIDC role assumption largely works like the `--terragrunt-iam-role` flag, with the addition of the `--terragrunt-iam-web-identity-token` flag. One special aspect of the `--terragrunt-iam-web-identity-token` flag is that it can use both a token, and the path to a file containing the token.

As a command line argument:

```bash
terragrunt apply --terragrunt-iam-role "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME" --terragrunt-iam-web-identity-token "$TOKEN"
```

As environment variables:

```bash
export TERRAGRUNT_IAM_ROLE="arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
export TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN="$TOKEN"
terragrunt apply
```

In the Terragrunt configuration:

```hcl
iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
iam_web_identity_token = get_env("AN_OIDC_TOKEN")
```

## Auth provider command

Finally, there is also a special flag that allows you to use an external command to provide the role assumption credentials. This is the most powerful and flexible option for setting up Terragrunt authentication, but it does require a bit more setup.

This technique is especially useful in the following circumstances:

- In a CI/CD pipelines, where you might want to use different role assumption mechanisms for different stages of the pipeline (like a read-only, plan role during pull request open, and a read-write, apply role during merge).
- On a shared development repository, where you might want to use different roles for different developers, or even different roles for the same developer, depending on the task at hand.
- In a setup where units in different accounts depend on each other, and you want to assume a different role for each account.

The [`--terragrunt-auth-provider-cmd`](/docs/reference/cli-options/#terragrunt-auth-provider-cmd) flag allows you to specify a command that can be executed by Terragrunt to fetch credentials at runtime.

```bash
terragrunt apply --terragrunt-auth-provider-cmd /path/to/auth-script.sh
```

As with all other flags, you can also set this as an environment variable:

```bash
export TERRAGRUNT_AUTH_PROVIDER_CMD="/path/to/auth-script.sh"
terragrunt apply
```

When Terragrunt executes this script, it will expect a response in STDOUT that obeys the following schema:

```json
{
  "awsCredentials": {
    "ACCESS_KEY_ID": "",
    "SECRET_ACCESS_KEY": "",
    "SESSION_TOKEN": ""
  },
  "awsRole": {
    "roleARN": "",
    "sessionName": "",
    "duration": 0,
    "webIdentityToken": ""
  },
  "envs": {
    "ANY_KEY": ""
  }
}
```

All of the top-level objects are optional, and you can provide multiple.

- `awsCredentials` is the standard AWS credential object, which can be used to set the `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and (optionally) `AWS_SESSION_TOKEN` environment variables before running OpenTofu/Terraform.
- `awsRole` is the role assumption object, which can be used to dynamically perform role assumption on the `roleARN` role with the `sessionName` session name, for a `duration` of time, and with a `webIdentityToken` if needed. Terragrunt will automatically refresh this role assumption when the duration expires.
- `envs` is a map of environment variables that will be set before running OpenTofu/Terraform.

Given that the working directory of Terragrunt execution is the same as the command, you can author logic in your script to determine which credentials are appropriate to return based on the context of the Terragrunt run.

This feature is integrated with the [Gruntwork Pipelines](https://www.gruntwork.io/platform/pipelines) product to provide a secure and flexible way to manage assumption of different roles in different accounts based on context.

## Required Permissions

You are ultimately responsible for ensuring that the IAM role you are assuming has the minimal and necessary permissions required to perform the activity you are attempting.

At a minimum, however there is some guidance that you can follow for ensuring that you have sufficient permissions.

Granting the following permissions on an IAM role:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowAllDynamoDBActionsOnAllTerragruntTables",
            "Effect": "Allow",
            "Action": "dynamodb:*",
            "Resource": [
                "arn:aws:dynamodb:*:1234567890:table/terragrunt*"
            ]
        },
        {
            "Sid": "AllowAllS3ActionsOnTerragruntBuckets",
            "Effect": "Allow",
            "Action": "s3:*",
            "Resource": [
                "arn:aws:s3:::terragrunt*",
                "arn:aws:s3:::terragrunt*/*"
            ]
        }
    ]
}
```

Will grant Terragrunt more than enough permissions to perform what it needs to do in AWS (replacing `1234567890` with your AWS account ID, and `terragrunt*` with the desired names of your Terragrunt resources).

Note that these permissions might be too broad for your circumstances, however. A more minimal policy might look like the following:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowCreateAndListS3ActionsOnSpecifiedTerragruntBucket",
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketVersioning",
                "s3:GetBucketAcl",
                "s3:GetBucketLogging",
                "s3:CreateBucket",
                "s3:PutBucketPublicAccessBlock",
                "s3:PutBucketTagging",
                "s3:PutBucketPolicy",
                "s3:PutBucketVersioning",
                "s3:PutEncryptionConfiguration",
                "s3:PutBucketAcl",
                "s3:PutBucketLogging",
                "s3:GetEncryptionConfiguration",
                "s3:GetBucketPolicy",
                "s3:GetBucketPublicAccessBlock",
                "s3:PutLifecycleConfiguration",
                "s3:PutBucketOwnershipControls"
            ],
            "Resource": "arn:aws:s3:::BUCKET_NAME"
        },
        {
            "Sid": "AllowGetAndPutS3ActionsOnSpecifiedTerragruntBucketPath",
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:GetObject"
            ],
            "Resource": "arn:aws:s3:::BUCKET_NAME/some/path/here"
        },
        {
            "Sid": "AllowCreateAndUpdateDynamoDBActionsOnSpecifiedTerragruntTable",
            "Effect": "Allow",
            "Action": [
                "dynamodb:PutItem",
                "dynamodb:GetItem",
                "dynamodb:DescribeTable",
                "dynamodb:DeleteItem",
                "dynamodb:CreateTable"
            ],
            "Resource": "arn:aws:dynamodb:*:*:table/TABLE_NAME"
        }
    ]
}
```

As you can see, the permissions are getting locked down, and the risk you run by adopting these permissions is that you might not realize that you need certain permissions until you run into an error. It's generally a best practice to start with permissions that are too narrow, and expand them as necessary.

Additionally, while Terragrunt _can_ provision the S3 bucket and DynamoDB table it uses for S3 state storage, it doesn't _need_ to. You can create these resources outside of Terragrunt, then grant Terragrunt permissions to interact with them (but not create them). A policy that allows this would look like the following:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": [
                "s3:GetBucketLocation",
                "s3:List*"
            ],
            "Resource": [
                "arn:aws:s3:::<BucketName>"
            ],
            "Effect": "Allow"
        },
        {
            "Action": [
                "s3:DeleteObject",
                "s3:GetObject",
                "s3:PutObject",
                "s3:ListBucket"
            ],
            "Resource": [
                "arn:aws:s3:::<BucketName>/*"
            ],
            "Effect": "Allow"
        },
        {
            "Sid": "AllowCreateAndUpdateDynamoDBActionsOnSpecifiedTerragruntTable",
            "Effect": "Allow",
            "Action": [
                "dynamodb:PutItem",
                "dynamodb:GetItem",
                "dynamodb:DescribeTable",
                "dynamodb:DeleteItem",
            ],
            "Resource": "arn:aws:dynamodb:*:*:table/TABLE_NAME"
        }
    ]
}
```

You'll want to make sure that you set configurations like `skip_bucket_versioning` in [remote_state](/docs/reference/config-blocks-and-attributes/#remote_state) to prevent Terragrunt from attempting to validate the bucket or table is in the proper configuration without requisite permissions.
