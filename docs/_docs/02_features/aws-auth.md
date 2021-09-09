---
layout: collection-browser-doc
title: AWS Auth
category: features
categories_url: features
excerpt: Learn how the Terragrunt works with AWS Credentials and AWS IAM policies.
tags: ["AWS"]
order: 260
nav_title: Documentation
nav_title_link: /docs/
---

## AWS credentials

Terragrunt uses the official [AWS SDK for Go](https://aws.amazon.com/sdk-for-go/), which means that it will automatically load credentials using the [AWS standard approach](https://aws.amazon.com/blogs/security/a-new-and-standardized-way-to-manage-credentials-in-the-aws-sdks/). If you need help configuring your credentials, please refer to the [Terraform docs](https://www.terraform.io/docs/providers/aws/#authentication).

## AWS IAM policies

Your AWS user must have an [IAM policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html) which grants permissions for interacting with DynamoDB and S3. Terragrunt will automatically create the configured DynamoDB tables and S3 buckets for storing remote state if they do not already exist.

The following is an example IAM policy for use with Terragrunt. The policy grants the following permissions:

  - all DynamoDB permissions in all regions for tables used by Terragrunt

  - all S3 permissions for buckets used by Terragrunt

Before using this policy, make sure to replace `1234567890` with your AWS account id and `terragrunt*` with your organization’s naming convention for AWS resources for Terraform remote state.

``` json
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

For a more minimal policy, for example when using a single bucket and DynamoDB table for multiple Terragrunt users, you can use the following. Be sure to replace `BUCKET_NAME` and `TABLE_NAME` with the S3 bucket name and DynamoDB table name respectively.

``` json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowCreateAndListS3ActionsOnSpecifiedTerragruntBucket",
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketVersioning",
                "s3:GetObject",
                "s3:GetBucketAcl",
                "s3:GetBucketLogging",
                "s3:CreateBucket",
                "s3:PutObject",
                "s3:PutBucketPublicAccessBlock",
                "s3:PutBucketTagging",
                "s3:PutBucketPolicy",
                "s3:PutBucketVersioning",
                "s3:PutEncryptionConfiguration",
                "s3:PutBucketAcl",
                "s3:PutBucketLogging"
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

When the above is applied to an IAM user it will restrict them to creating the DynamoDB table if it doesn’t already exist and allow updating records for state locking, and for the S3 bucket will allow creating the bucket if it doesn’t already exist and only write files to the specified path.

If you are only given access to an externally created Bucket you will need at least this IAM policy to be granted to your account:

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
                    "arn:aws:s3:::<BucketName>/\*"
                ],
                "Effect": "Allow"
            }
        ]
    }

and you will need to set the flag `skip_bucket_versioning` to true (only bucket owners can check versioning status on an S3 Bucket)
