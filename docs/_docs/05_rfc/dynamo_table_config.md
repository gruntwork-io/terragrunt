---
layout: collection-browser-doc
title: Custom state configuration
category: RFC
categories_url: rfc
excerpt: Allow further customization of Terraform Lock table for S3 Remote State.
tags: ["rfc", "contributing", "community"]
order: 503
nav_title: Documentation
nav_title_link: /docs/
---

<!-- markdownlint-disable -->

# Allow further customization of Terraform Lock table for S3 Remote State

**STATUS**: In proposal

## Background

When Terragrunt creates a DynamoDB table, it sets the throughput to the default,
`PROVISIONED`. Provisioned is good for stable throughput, but Terraform largely
works with variable workloads, often going unused.

AWS also offers a `PAY_PER_REQUEST` billing mode that charges based on the
number of requests to the lock table. The pricing is dirt cheap, and it will
further reduce the costs associated with using Terragrunt to manage Terraform's
base requirements.

Terraform has solved this problem: It involves creating and managing the
resource manually. But, if Terragrunt is going to be managing the table, it
should allow for additional configuration to be layered on top of it via
Terragrunt.

## Proposed solution

### `dynamodb_table_config` block in `remote_state.config`

Because our company has a need for it, I've implemented a solution in my own
fork that accomplishes this.

https://github.com/ThatGerber/terragrunt/tree/add-dynamodb-billing-mode

The `dynamodb_table_config` block accepts several arguments that are forwarded
through to the DynamoTableInput object, pulling in additional values from the
parent config as defaults.

To receive the full values, I had to write a few interfaces for the methods
being called (like `GetLockTableName`) and change the function calls to accept
the interfaces instead of strings.

### `dynamodb_table_config` block that populates `dynamodb.CreateTableInput` struct

Another option is to create a config attribute that accepts any arguments that
would be sent to `CreateTable` or `UpdateTable` map the values through to the
methods when they're being applied.

## References

- [DynamoDB Pricing](https://aws.amazon.com/dynamodb/pricing/)
- [DynamoDB Billing Mods](https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_BillingModeSummary.html)
