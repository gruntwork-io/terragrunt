# Terragrunt

Terragrunt is a thin wrapper for the [Terraform client](https://www.terraform.io/) that provides simple locking 
mechanisms so that multiple people can collaborate on the same [Terraform state](https://www.terraform.io/docs/state/) 
without overwriting each other's changes. You can choose from one of the supported locking mechanisms:
 
1. **Git**: Use your version control system to acquire and release locks. This is a totally free solution that should
   work for most teams.
1. **DynamoDB**: Use Amazon's [DynamoDB](https://aws.amazon.com/dynamodb/) to acquire and release locks. This is a 
   great option for teams already using AWS. DynamoDB is part of the [AWS free 
   tier](https://aws.amazon.com/dynamodb/pricing/), so this option should also be free.
 
## Motivation

When you use Terraform to provision infrastructure, it records the state of your infrastructure in [state 
files](https://www.terraform.io/docs/state/). In order to make changes to your infrastructure, everyone on your
team needs access to these state files. You could check the files into version control (not a great idea, as the state
files may contain secrets) or use a supported [remote state 
backend](https://www.terraform.io/docs/state/remote/index.html) to store the state files in a shared location such as 
[S3](https://www.terraform.io/docs/state/remote/s3.html), 
[Consul](https://www.terraform.io/docs/state/remote/consul.html), 
or [etcd](https://www.terraform.io/docs/state/remote/etcd.html). The problem is that none of these options provide 
*locking*, so if two team members run `terraform apply` on the same state files at the same time, they may overwrite 
each other's changes. The official solution to this problem is to use [Hashicorp's 
Atlas](https://www.hashicorp.com/atlas.html), but that requires using a SaaS platform for all Terraform operations and
can cost a lot of money.

The goal of Terragrunt is to provide simple locking mechanisms that are free or very inexpensive so that multiple people 
can safely collaborate on Terraform state.

## Install

1. Install [Terraform](https://www.terraform.io/).
1. Install [jq](https://stedolan.github.io/jq/).
1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases), downloading
   the binary for your OS, and adding it to your PATH.

## Quick start

Go into the folder with your Terraform templates and create a `.terragrunt` file. This file uses the same
[HCL](https://github.com/hashicorp/hcl) syntax as Terraform and is used to configure Terragrunt and tell it how to do
locking.
 
If you want to use Git for locking (see [Locking using Git](#locking-using-git)), `.terragrunt` should have the
following contents:

```hcl
lockType = "git"
stateFileId = "my-app"
```

If you wish to use DynamoDB for locking (see [Locking using DynamoDB](#locking-using-dynamodb)), `.terragrunt` should
have the following contents:

```hcl
lockType = "dynamodb"
stateFileId = "my-app"
```

Now everyone on your team can use Terragrunt to run all the standard Terraform commands:

```bash
terragrunt get
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
```

Terragrunt forwards most commands directly to Terraform. However, for the `apply` and `destroy` commands, it will first 
acquire a locking using either [Git](#locking-using-git) or [DynamoDB](#locking-using-dynamodb), as described below.

## Locking using Git

Terragrunt can use Git as a poor-man's locking solution by trying to create a file in a repo to acquire a lock (if it
fails, that means someone else already has the lock) and deleting that file to release the lock. Git is a distributed 
version control system, which would normally make locking impossible, but we rely on the fact that most teams have a
single Git server (e.g. GitHub, BitBucket) that is used as the central source of truth and can therefore be used as the
single source of locking. 

#### Git locking prerequisites

To use Git for locking, you must:

1. Store your Terraform templates in a Git repo (always a good idea).
1. Make sure the current user has permissions to read and write from the repo using `git pull` and `git push`. 

#### Git locking configuration
 
For Git locking, Terragrunt supports the following settings in `.terragrunt`:

```hcl
lockType = "git"
stateFileId = "my-app"

gitLock = {
  lockBranch = "terragrunt_locks"
  remoteName = "origin"
}
```

* `lockType`: (Required) Must be set to `git`.
* `stateFileId`: (Required) A unique id for the state file for these Terraform templates. Many teams have more than
  one set of templates, and therefore more than one state file, so this setting is used to disambiguate locks for one 
  state file from another.
* `lockBranch`: (Optional) The branch Terragrunt should use for creating and deleting lock files. Default:
  `terragrunt_locks`.
* `remoteName`: (Optional) The name of the remote repository that should be used as the central source of truth, and
  therefore locking. Default: `origin`.
 
#### How Git locking works
 
When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Clone your local checkout into a temporary directory under `/tmp/terragrunt/YOUR-REPO`. Terragrunt will make all of
   its changes and commits in this `/tmp/terragrunt/YOUR-REPO` folder to ensure it doesn't mess up your local checkout.
1. Check out the `terragrunt_locks` branch in the `/tmp/terragrunt/YOUR-REPO` folder, creating the branch from master
   if it doesn't already exist. Terragrunt does all its commits in this branch so it doesn't dirty the commit history 
   of your other branches.
1. Read the `stateFileId` value from your `.terragrunt` file and create a *Lock File* of the same name in the
   `terragrunt_locks` branch. The Lock File contents will include useful metadata about the lock, such as who created it
   (e.g. your username) and when.
1. Try to push the Lock File to the `terragrunt_locks` branch.
    1. If the push succeeds, it means we have a lock!
    1. If the push does not succeed, it means someone else has a lock. Keep retrying every 30 seconds until we get a 
       lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the Lock File from the `terragrunt_locks` branch to release the lock.
1. Delete `/tmp/terragrunt/YOUR-REPO`.

## Locking using DynamoDB

Terragrunt can use Amazon's [DynamoDB](https://aws.amazon.com/dynamodb/) as a slightly more sophisticated mechanism
of acquiring and releasing locks. DynamoDB supports 
[strongly consistent reads](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.DataConsistency.html)
as well as [conditional writes](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.SpecifyingConditions.html),
which are all the primitives we need for a very basic distributed lock system. It's also part of [AWS's free
tier](https://aws.amazon.com/dynamodb/pricing/), and given the tiny amount of data we are working with and the 
relatively small number of times per day you're likely to run Terraform, it should be a free option for teams already
using AWS.

#### DynamoDB locking prerequisites

To use DynamoDB for locking, you must:

1. Already have an AWS account.
1. Set your AWS credentials in the environment using one of the following options:
    1. Set your credentials as the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
    1. Run `aws configure` and fill in the details it asks for.
    1. Run Terragrunt on an EC2 instance with an IAM Role.
1. Your AWS user must have an [IAM 
   policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html) 
   granting all DynamoDB actions (`dynamodb:*`) on the table `terragrunt_locks` (see the
   [DynamoDB locking configuration](#dynamodb-locking-configuration) for how to configure this table name). Here is an 
   example IAM policy that grants the necessary permissions on the `terragrunt_locks` in region `us-west-2` for
   an account with account id `1234567890`:

    ```json
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Sid": "",
        "Effect": "Allow",
        "Action": "dynamodb:*",
        "Resource": "arn:aws:dynamodb:us-west-2:1234567890:table/terragrunt_locks"
      }]
    }
    ```

#### DynamoDB locking configuration
 
For DynamoDB locking, Terragrunt supports the following settings in `.terragrunt`:

```hcl
lockType = "dynamodb"
stateFileId = "my-app"

dynamoLock = {
  awsRegion = "us-east-1"
  tableName = "terragrunt_locks"
}
```

* `lockType`: (Required) Must be set to `dynamodb`.
* `stateFileId`: (Required) A unique id for the state file for these Terraform templates. Many teams have more than
  one set of templates, and therefore more than one state file, so this setting is used to disambiguate locks for one 
  state file from another.
* `awsRegion`: (Optional) The AWS region to use. Default: `us-east-1`.
* `tableName`: (Optional) The name of the table in DynamoDB to use to store lock information. Default:
  `terragrunt_locks`.

#### How DynamoDB locking works

When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Create the `terragrunt_locks` if it doesn't already exist.
1. Try to write an item to the `terragrunt_locks` with `stateFileId` equal to the id specified in your
   `.terragrunt` file. This item will include useful metadata about the lock, such as who created it (e.g. your 
   username) and when. 
1. Note that the write is a conditional write that will fail if an item with the same `stateFileId` already exists.
    1. If the write succeeds, it means we have a lock!
    1. If the write does not succeed, it means someone else has a lock. Keep retrying every 30 seconds until we get a 
       lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the item from the `terragrunt_locks` to release the lock.
 
## Cleaning up old locks

If you shut down Terragrunt (e.g. via `CTRL+C`) before it releases a lock, the lock hangs around forever, and will 
prevent future changes to your state files. All Terragrunt locking mechanisms contain useful metadata, such as who
acquired the lock and when, which can be useful in determining if a lock is stale and needs to be cleaned up.

To clean up old locks, you can use the `release-lock` command:

```
terragrunt release-lock
Are you sure you want to forcibly remove the lock for stateFileId "my-app"? (y/n):
```

If for some reason this doesn't work, you can also clean up locks manually:

1. For Git locking, delete the lock file in the `terragrunt_locks` branch in Git (see [How Git locking
   works](#how-git-locking-works)).
1. For DynamoDB, delete the lock item in the `terragrunt_locks` (see [How DynamoDB locking
   works](#how-dynamodb-locking-works)).

## TODO

* Automated tests
* CI job to run the tests and build and publish new binaries
* Implement best-practices in Terragrunt, such as checking if all changes are committed, calling `terraform get`,
  calling `terraform configure`, etc.