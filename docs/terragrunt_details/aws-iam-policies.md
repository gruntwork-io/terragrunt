---
title: AWS IAM Policies
layout: single
author_profile: true
sidebar:
  nav: "docs"
---

Your AWS user must have an [IAM
policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html)
which grants permissions for interacting with DynamoDB and S3. Terragrunt will automatically create
the configured DynamoDB tables and S3 buckets for storing remote state if they do not already exist.

The following is an example IAM policy for use with Terragrunt. The policy grants the following permissions:

* all DynamoDB permissions in all regions for tables used by Terragrunt
* all S3 permissions for buckets used by Terragrunt

Before using this policy, make sure to replace `1234567890` with your AWS account id and `terragrunt*` with
your organization's naming convention for AWS resources for Terraform remote state.

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
