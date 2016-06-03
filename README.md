# Terragrunt

Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that supports locking, encryption, and
automatic management for Terraform state files.

1. **Locking**. Terragrunt can use either your existing Git repository (e.g. GitHub, BitBucket, etc) or Amazon
   DynamoDB as a distributed lock for your Terraform state files, ensuring team members working on those state files do
   not overwrite each other's changes.
1. **Encryption**. Terragrunt can use Amazon Key Management Service (KMS) to encrypt and decrypt your Terraform state
   files, passing the unencrypted data to Terraform solely in-memory, and always encrypting the data before writing it
   to disk.
1. **Automatic state management**: Terragrunt can automatically store and retrieve your Terraform state files from a
   central location accessible to your whole team, such as Git or S3.

## Motivation

When you use Terraform to provision infrastructure, it records the state of your infrastructure in [state 
files](https://www.terraform.io/docs/state/). In order to make changes to your infrastructure, everyone on your
team needs access to these state files. You could check the files into version control or use a supported [remote state
backend](https://www.terraform.io/docs/state/remote/index.html) to store the state files in a shared location such as 
[S3](https://www.terraform.io/docs/state/remote/s3.html), 
[Consul](https://www.terraform.io/docs/state/remote/consul.html), 
or [etcd](https://www.terraform.io/docs/state/remote/etcd.html). All of these options have several problems:

1. They do not provide *locking*. If two team members run `terraform apply` on the same state files at the same
   time, they may overwrite each other's changes. The official solution to this problem is to use [Hashicorp's
   Atlas](https://www.hashicorp.com/atlas.html), but that can be a fairly expensive option, and it requires you to use
   a SaaS platform for all Terraform operations.
1. They do not provide *encryption*. Terraform state files may contain secrets, such as database passwords, and by
   default, Terraform stores them in plain text, both on your local file system and in most of the supported remote
   stores.
1. They are *error prone*. Very often, you do a fresh checkout of a bunch of Terraform templates from version control,
   forget to enable remote state storage before applying them, and end up creating a bunch of duplicate resources.
   Or maybe you do remember to enable remote state storage, but you use the wrong configuration (e.g. the wrong S3
   bucket name or key) and you end up overwriting the state for a totally different set of templates. Or perhaps you're
   storing your state files in Git and you forgot to do a `git pull` before running `terraform apply` and you end up
   with conflicting state files when you try to `git push`.

The goal of Terragrunt is to take Terraform, which is a fantastic tool, and make it even better for teams by providing
locking, encryption, and automatic state management.

## Install

1. Install [Terraform](https://www.terraform.io/) and make sure it is in your PATH.
1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases), downloading
   the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.

## Quick start

Go into the folder with your Terraform templates and create a `.terragrunt` file. This file uses the same
[HCL](https://github.com/hashicorp/hcl) syntax as Terraform. Here is an example `.terragrunt` file that configures
Terragrunt to use [Git for locking](#locking-using-git), [KMS for encryption](#encryption-using-kms), and to
[automatically store the encrypted state files in Git](#automatic-state-management):

```hcl
# Configure Terragrunt to use a central Git repo for locking
gitLock = {
  stateFileId = "my-app"
}

# Configure Terragrunt to encrypt your tfstate files using KMS
kmsEncryption = {
  keyId = "alias/my-terragrunt-key"
}

# Configure Terragrunt to automatically store Terraform state files in Git
remoteState = {
  backend = "git"
  backendConfigs = {
    key = "terraform.tfstate"
  }
}
```

And here is an example of how you can configure Terragrunt to use [DynamoDB for locking](#locking-using-dynamodb) and to
[automatically store state files in an encrypted S3 bucket](#automatic-state-management):

```hcl
# Configure Terragrunt to use DynamoDB for locking
dynamoDbLock = {
  stateFileId = "my-app"
}

# Configure Terragrunt to automatically store Terraform state files in an S3 bucket
remoteState = {
  backend = "s3"
  backendConfigs = {
    encrypt = "true"
    bucket = "my-bucket"
    key = "terraform.tfstate"
    region = "us-east-1"
  }
}
```

Once you check a `.terragrunt` file into source control, everyone on your team can use Terragrunt to run all the
standard Terraform commands:

```bash
terragrunt get
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
```

Terragrunt forwards almost all commands, arguments, and options directly to Terraform, using whatever version of
Terraform you already have installed. However, before running Terraform, Terragrunt will ensure you have the latest
state based on the `remoteState` setting in the `.terragrunt` file. Moreover, for the `apply` and `destroy` commands,
Terragrunt will first try to acquire a lock using either Git or DynamoDB.

```
terragrunt apply
[terragrunt] 2016/05/30 16:55:28 Configuring remote state for the s3 backend
[terragrunt] 2016/05/30 16:55:28 Running command: terraform remote config -backend s3 -backend-config=key=terraform.tfstate -backend-config=region=us-east-1 -backend-config=encrypt=true -backend-config=bucket=my-bucket
Initialized blank state with remote state enabled!
[terragrunt] 2016/05/30 16:55:29 Attempting to acquire lock for state file my-app in DynamoDB
[terragrunt] 2016/05/30 16:55:30 Attempting to create lock item for state file my-app in DynamoDB table terragrunt_locks
[terragrunt] 2016/05/30 16:55:30 Lock acquired!
[terragrunt] 2016/05/30 16:55:30 Running command: terraform apply
terraform apply

aws_instance.example: Creating...
  ami:                      "" => "ami-0d729a60"
  instance_type:            "" => "t2.micro"

[...]

Apply complete! Resources: 1 added, 0 changed, 0 destroyed.

[terragrunt] 2016/05/27 00:39:19 Attempting to release lock for state file my-app in DynamoDB
[terragrunt] 2016/05/27 00:39:19 Lock released!
```

For details on how locking and state management work and the trade-offs between the different options, see the
following:

* [Locking using Git](#locking-using-git)
* [Locking using DynamoDB](#locking-using-dynamodb)
* [Encryption using KMS](#encryption-using-kms)
* [Automatic state management](#automatic-state-management)
* [Recommended configurations](#recommended-configurations)

## Locking using Git

Terragrunt can use Git to acquire and release locks. Although Git is a distributed version control system, most teams
have a central repo they use as their "source of truth", such as GitHub or BitBucket. Terragrunt can use this
centralized repo as a lock by trying to create a file in the repo to acquire a lock (if it fails, that means someone
else already has the lock) and deleting that file to release the lock.

One of the benefits of Git locking is that your commit history becomes a log of all Terraform changes that have
happened. The commit message for each lock will contain useful metadata about who acquired the lock and when, so you can
use `git log` to debug deployment problems. Moreover, if you use Git locking with [Git state
management](#automatic-state-management-with-git), Terragrunt will even store the [plan
file](https://www.terraform.io/docs/commands/plan.html) for each deployment, so you'll be able to go back and see
exactly what changes were applied for each deployment.

#### Git locking prerequisites

To use Git for locking, you must:

1. Make sure the current user has permissions to read and write from the repo using `git pull` and `git push`.

#### Git locking configuration

For Git locking, Terragrunt supports the following settings in `.terragrunt`:

```hcl
gitLock = {
  stateFileId = "my-app"
  branchName = "terragrunt"
  remoteName = "origin"
}
```

* `stateFileId`: (Required) A unique id for the state file for these Terraform templates. Many teams have more than
  one set of templates, and therefore more than one state file, so this setting is used to disambiguate locks for one
  state file from another.
* `branchName`: (Optional) The branch Terragrunt should use for creating and deleting lock files. Storing lock files
  in a separate branch keeps the commit history on your other branches clean. Default: `terragrunt`.
* `remotename`: (Optional) The name of the remote repository from the `git remote` command that should be used as the
  central source of truth, and therefore locking. Default: `origin`.

#### How Git locking works

When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Clone your local checkout into a temporary directory under `/tmp/terragrunt/YOUR-REPO`. Terragrunt will make all of
   its changes and commits in this `/tmp/terragrunt/YOUR-REPO` folder to ensure it doesn't mess up your local checkout.
   If the folder already exists, Terragrunt will reuse it.
1. Check out the `terragrunt` branch in the `/tmp/terragrunt/YOUR-REPO` folder, creating the branch from master
   if it doesn't already exist. Terragrunt does all its commits in this branch so it doesn't dirty the commit history
   of your other branches.
1. `git pull` the latest code for the `terragrunt` branch.
1. Create a *Lock File* in the `terragrunt` branch. The Lock File will be named using the `stateFileId` setting your
   your `.terragrunt` configuration.
1. Commit the Lock File file to the `terragrunt` branch with a commit message that has useful metadata, such as
   "User XXX is using Terragrunt to deploy my-app on date YYY based on a plan generated from commit ZZZ."
1. Try to push the Lock File to the `terragrunt` branch.
    1. If the push succeeds, it means we have a lock!
    1. If the push does not succeed, it means someone else already created that file, and therefore already has a lock.
    Keep retrying until we get a lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the Lock File from the `terragrunt` branch to release the lock.

## Locking using DynamoDB

Terragrunt can use Amazon's [DynamoDB](https://aws.amazon.com/dynamodb/) to acquire and release locks. DynamoDB supports
[strongly consistent reads](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.DataConsistency.html)
as well as [conditional writes](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.SpecifyingConditions.html),
which are all the primitives we need for a very basic distributed lock system. It's also part of [AWS's free
tier](https://aws.amazon.com/dynamodb/pricing/), and given the tiny amount of data we are working with and the 
relatively small number of times per day you're likely to run Terraform, it _should_ be a free option for teams already
using AWS. We take no responsibility for any charges you may incur.

#### DynamoDB locking prerequisites

To use DynamoDB for locking, you must:

1. Set your AWS credentials in the environment using one of the following options:
    1. Set your credentials as the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
    1. Run `aws configure` and fill in the details it asks for.
    1. Run Terragrunt on an EC2 instance with an IAM Role.
1. Your AWS user must have an [IAM 
   policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html) 
   granting all DynamoDB actions (`dynamodb:*`) on the table `terragrunt_locks` (see the
   [DynamoDB locking configuration](#dynamodb-locking-configuration) for how to configure this table name). Here is an 
   example IAM policy that grants the necessary permissions on the `terragrunt_locks` table in region `us-west-2` for
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
dynamoDbLock = {
  stateFileId = "my-app"
  awsRegion = "us-east-1"
  tableName = "terragrunt_locks"
  maxLockRetries = 360
}
```

* `stateFileId`: (Required) A unique id for the state file for these Terraform templates. Many teams have more than
  one set of templates, and therefore more than one state file, so this setting is used to disambiguate locks for one 
  state file from another.
* `awsRegion`: (Optional) The AWS region to use. Default: `us-east-1`.
* `tableName`: (Optional) The name of the table in DynamoDB to use to store lock information. Default:
  `terragrunt_locks`.
* `maxLockRetries`: (Optional) The maximum number of times to retry acquiring a lock. Terragrunt waits 10 seconds
  between retries. Default: 360 retries (one hour).

#### How DynamoDB locking works

When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Create the `terragrunt_locks` table if it doesn't already exist.
1. Try to write an item to the `terragrunt_locks` table with `stateFileId` equal to the id specified in your
   `.terragrunt` file. This item will include useful metadata about the lock, such as who created it (e.g. your 
   username) and when. 
1. Note that the write is a conditional write that will fail if an item with the same `stateFileId` already exists.
    1. If the write succeeds, it means we have a lock!
    1. If the write does not succeed, it means someone else has a lock. Keep retrying every 10 seconds until we get a
       lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the item from the `terragrunt_locks` table to release the lock.

## Encryption using KMS

Terragrunt can use Amazon's [Key Management Service (KMS)](https://aws.amazon.com/kms/) to encrypt and decrypt your
state files. It does this transparently to Terraform, ensuring that the data on disk is always encrypted, and the
unencrypted data is only available in memory. KMS is part of [AWS's free tier](https://aws.amazon.com/kms/pricing/),
so the requests will be free. However, you will need to create a master key, which costs $1/month.

**Note**: KMS encryption currently only works with state files stored locally or in Git (both of which are safe since
the data is encrypted). It does NOT work with any of the other remote state stores, such as S3. See [automatic state
management](#automatic-state-management) for more info.

#### KMS encryption prerequisites

To use KMS encryption, you must:

1. Set your AWS credentials in the environment using one of the following options:
    1. Set your credentials as the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
    1. Run `aws configure` and fill in the details it asks for.
    1. Run Terragrunt on an EC2 instance with an IAM Role.
1. Your AWS user must have a [key policy](http://docs.aws.amazon.com/kms/latest/developerguide/key-policies.html)
   granting you `kms:Decrypt` and `kms:GenerateDataKey*` on the master key you specify in the `.terragrunt` config.
   Here is an example key policy that grants the necessary permissions for user `jon` an account with account id
   `1234567890`:

    ```json
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {"AWS": [
        "arn:aws:iam::1234567890:jon"
      ]},
      "Action": [
        "kms:Decrypt",
        "kms:GenerateDataKey*",
      ],
      "Resource": "*"
    }
    ```

#### KMS encryption configuration

For KMS encryption, Terragrunt supports the following settings in `.terragrunt`:

```hcl
kmsEncryption = {
  keyId = "alias/my-terragrunt-key"
  dataKeyFile = "terragrunt.data.key"
}
```

* `keyId`: (Required) The id of the KMS encryption master key to use. Can be a key ARN (e.g.
   `arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012`), globally unique key id (e.g.
   `12345678-1234-1234-1234-123456789012`), or alias name (e.g. `alias/my-terragrunt-key`).
* `dataKeyFile`: (Optional) The name of the file where the generated data key will be stored in encrypted format.
  Default: `terragrunt.data.key`.

#### How KMS encryption works

The very first time you run Terragrunt with KMS encryption enabled, it does the following:

1. Create a [Data Key](http://docs.aws.amazon.com/kms/latest/developerguide/concepts.html#data-keys) using KMS.
1. Use the Data Key to encrypt your Terraform state file.
1. Use KMS to encrypt the Data Key itself and write it to disk in a file called `terragrunt.data.key`.

After that, every time you run a Terragrunt command that needs your Terraform state files, Terragrunt does the
following:

1. Read the encrypted Data Key from `terragrunt.data.key`.
1. Decrypt the Data Key using KMS.
1. Use the decrypted Data Key to decrypt the Terraform state file (in memory, not on disk).
1. Call Terraform and set the `-state` parameter, which specifies where Terraform can find the original state file, and
   the `-state-out` parameter, which specifies where Terraform can write the updated state file, to custom file
   descriptors. Instead of pointing those file descriptors to actual files on disk, Terraform reads and writes to those
   file descriptors from memory (e.g. a bit like using `/dev/stdin` as a file descriptor).
1. When Terraform is done, encrypt the contents of the `-state-out` file descriptor using the Data Key and write that
   encrypted data into the Terraform state file.

#### Decrypting data

If you need to be able to see your Terraform state file unencrypted (e.g. for debugging), you can use the `decrypt`
command:

```bash
terragrunt decrypt terraform.tfstate
```

The command above will output the unencrypted contents of `terraform.tfstate` to stdout. Note that the `decrypt`
command works on any file encrypted with the KMS Data Key in `terragrunt.data.key`, including any plan files that were
saved if were are using [Automatic state management with Git](#automatic-state-management-with-git):

```bash
terragrunt decrypt deployment-05-06-16.plan
```

## Automatic state management

Terragrunt can automatically manage state storage for you, preventing manual errors such as forgetting to enable remote
state, using the wrong settings, or forgetting to pull the latest state before making changes. Terragrunt supports all
the same [remote state backends](https://www.terraform.io/docs/state/remote/) as Terraform (e.g. S3, Consul). It also
supports one new one, Git, which is documented in the [Automatic state management with
Git](#automatic-state-management-with-git) section.

#### Automatic state management prerequisites

Check out the [Terraform remote state backend docs](https://www.terraform.io/docs/state/remote/) for the requirements
to use a particular remote state backend. See the [Automatic state management with
Git](#automatic-state-management-with-git) docs for the requirements to use the Git backend.

#### Automatic state management configuration

For automatic state management, Terragrunt supports the following settings in `.terragrunt`:

```hcl
remoteState = {
  backend = "s3"
  backendConfigs = {
    key1 = "value1"
    key2 = "value2"
    key3 = "value3"
  }
}
```

* `backend`: (Required) The name of the remote state backend to use (e.g. git, s3, consul).
* `backendConfigs`: (Optional) A map of additional key/value pairs to pass to the backend. Each backend requires
  different key/value pairs, so consult the [Terraform remote state docs](https://www.terraform.io/docs/state/remote/)
  or the [Automatic state management with Git](#automatic-state-management-with-git) docs for details.

## Automatic state management with Git

Terragrunt can store your Terraform state files in Git, ensuring that you pull down the state file data from Git before
running any Terraform command, and pushing your updated state file data after the Terraform command finishes. Moreover,
Terragrunt can also store two other useful pieces of data:

1. The [Terraform plan file](https://www.terraform.io/docs/commands/plan.html) from each run.
1. The Terraform log output from each run.

Since Terraform state files, plan files, and log output may contain secrets, it is **highly recommended that you only
use this feature with [KMS encryption](#encryption-using-kms)**, which will ensure all stored data is encrypted.

The benefit of storing your state files, plan files, and log files in Git is that the full history of every Terraform
change—who made the change, when they made it, what resources were changed, and what actually happened when Terraform
applied the change—gets stored in your Git history, which makes it easier to debug deployment issues.

#### Automatic state management with Git prerequisites

To use automatic state management with Git, you must:

1. Make sure the current user has permissions to read and write from the repo using `git pull` and `git push`.

#### Automatic state management with Git configuration

For automatic state management with Git, Terragrunt supports the following settings in `.terragrunt`:

```hcl
remoteState = {
  backend = "git"
  backendConfigs = {
    key = "terraform.tfstate"
    storePlanFiles = true
    storeTerraformOutput = true
    branchName = "terragrunt"
    remoteName = "origin"
  }
}
```

* `backend`: (Required) Must be set to "git".
* `key`: (Optional) The name to use for the Terraform state file. If KMS encryption is enabled, this file will be
  encrypted. Default: `terraform.tfstate`.
* `storePlanFiles`: (Optional) Whether or not to store plan files for each run. Each one will get a timestamped name
  such as `terraform-plan-05-06-16-14:33:21+0200.plan`. If KMS encryption is enabled, this file will be encrypted.
  Default: `true`.
* `storeTerraformOutput`: (Optional) Whether or not to store Terraform log outputfor each run. Each one will get a
  timestamped name such as `terraform-output-05-06-16-14:33:21+0200.log`. If KMS encryption is enabled, this file will
  be encrypted. Default: `true`.
* `branchName`: (Optional) The name of the branch where the Terraform state files, plan files, and log output should
  be stored. Default: `terragrunt`.
* `remotename`: (Optional) The name of the remote repository from the `git remote` command where the Terraform state
  files, plan files, and log output should be stored. Default: `origin`.

#### How Automatic state management with Git works

When you run a Terraform command that needs Terraform state, Terragrunt will:

1. Clone your local checkout into a temporary directory under `/tmp/terragrunt/YOUR-REPO`. Terragrunt will make all of
   its changes and commits in this `/tmp/terragrunt/YOUR-REPO` folder to ensure it doesn't mess up your local checkout.
   If the folder already exists, Terragrunt will reuse it.
1. Check out the `terragrunt` branch in the `/tmp/terragrunt/YOUR-REPO` folder, creating the branch from master
   if it doesn't already exist. Terragrunt does all its commits in this branch so it doesn't dirty the commit history
   of your other branches.
1. `git pull` to ensure you have the latest Terraform state files.
1. If you're running `terragrunt apply` and `storePlanFiles` is enabled, run `terraform plan` and store the output in a
   plan file in the `terragrunt` branch. If KMS encryption is enabled, the plan file will be encrypted.
1. Run the Terraform command in your original checkout.
1. If `storeTerraformOutput` is enabled, store the Terraform output in a log file in the `terragrunt` branch. If KMS
   encryption is enabled, the log file will be encrypted.
1. `git commit` the state file, plan file, and log file to the `terragrunt` branch.
1. `git push` the `terragrunt` branch.

## Recommended configurations

So which locking system should you choose, Git or DynamoDB? And which state storage should you use, local, Git, S3,
or Consul? Should you enable encryption or not?

We recommend using one of the following two configurations:

1. Git locking, Git state management, and KMS encryption
1. DynamoDB locking and S3 state management

We discuss the trade-offs between them next.

#### Git locking, Git state management, and KMS encryption

To enable this configuration, use the following instructions:

1. [Locking using Git](#locking-using-git).
1. [Automatic state management with Git](#automatic-state-management-with-git).
1. [Encryption using KMS](#encryption-using-kms).

This configuration offers the following benefits:

1. You get a complete history of all deployments, including all the plan files that show what changes were applied and
   the log output from Terraform that show what happened when Terraform tried to apply those changes.
1. You get a complete history of all versions of your Terraform state file, so you can roll back to a previous version
   if anything goes wrong.
1. You get to leverage the version control system you're already using for locking and state management, which means
   you have fewer third party services to rely on, debug, maintain, secure, etc.
1. The Terraform state files, plan files, and log output are all encrypted, ensuring that your secrets are never
   written to disk in plain text.

This configuration also has some downsides:

1. Using Git for locking is a little hacky.
1. Using file-descriptor magic to ensure Terraform sees unencrypted data without ever writing that unencrypted data to
   disk is a little hacky.
1. Git is not one of the officially supported remote state backends for Terraform.

#### DynamoDB locking and S3 state management

To enable this configuration, use the following instructions:

1. [Locking using DynamoDB](#locking-using-dynamodb).
1. [Automatic state management](#automatic-state-management).
1. [S3 remote state backend](https://www.terraform.io/docs/state/remote/s3.html).
1. Make sure to enable encryption in the S3 remote state backend by setting `encrypt = "true"` in `.terragrunt`.
1. Make sure to [enable versioning](http://docs.aws.amazon.com/AmazonS3/latest/UG/enable-bucket-versioning.html) for
   your S3 bucket.

This configuration offers the following benefits:

1. DynamoDB is a reliable, tested data store that is a reasonable choice for distributed locking
   ([example](https://r.32k.io/locking-with-dynamodb)).
1. S3 is an officially supported remote state backend for Terraform.
1. With versioning enabled in your S3 bucket, you have the full history of all changes to the state files and you can
   roll back to a previous version if anything goes wrong.
1. With encryption enabled for the S3 backend, your Terraform state files are encrypted in motion (since Terraform uses
   SSL) and at rest in S3.

This configuration also has some downsides:

1. Your Terraform state files are unencrypted on any computer that uses Terraform, including your local dev computer
   and any CI box.
1. With DynamoDB and S3, you have more 3rd party services to rely on, debug, maintain, secure, etc.
1. It's easier to lock down a KMS key than an S3 bucket.
1. You don't have the full history of Terraform plan files and logs stored in Git.

## Cleaning up old locks

If Terragrunt is shut down before it releases a lock (e.g. via `CTRL+C` or a crash), the lock might not be deleted, and
will prevent future changes to your state files. To clean up old locks, you can use the `release-lock` command:

```
terragrunt release-lock
Are you sure you want to forcibly remove the lock for stateFileId "my-app"? (y/n): y
```

## Developing terragrunt

#### Running locally

To run Terragrunt locally, use the `go run` command:

```bash
go run main.go plan
```

#### Running tests

**Note**: The tests in the `dynamodb` folder for Terragrunt run against a real AWS account and will add and remove
real data from DynamoDB. DO NOT hit `CTRL+C` while the tests are running, as this will prevent them from cleaning up
temporary tables and data in DynamoDB. We are not responsible for any charges you may incur.

Before running the tests, you must configure your AWS credentials as explained in the [DynamoDB locking
prerequisites](#dynamodb-locking-prerequisites) section.

To run all the tests:

```bash
go test -v -parallel 128 $(glide novendor)
```

To run only the tests in a specific package, such as the package `remote`:

```bash
cd remote
go test -v -parallel 128
```

And to run a specific test, such as `TestToTerraformRemoteConfigArgsNoBackendConfigs` in package `remote`:

```bash
cd remote
go test -v -parallel 128 -run TestToTerraformRemoteConfigArgsNoBackendConfigs
```

#### Debug logging

If you set the `TERRAGRUNT_DEBUG` environment variable to "true", the stack trace for any error will be printed to
stdout when you run the app.

#### Error handling

In this project, we try to ensure that:

1. Every error has a stacktrace. This makes debugging easier.
1. Every error generated by our own code (as opposed to errors from Go built-in functions or errors from 3rd party
   libraries) has a custom type. This makes error handling more precise, as we can decide to handle different types of
   errors differently.

To accomplish these two goals, we have created an `errors` package that has several helper methods, such as
`errors.WithStackTrace(err error)`, which wraps the given `error` in an Error object that contains a stacktrace. Under
the hood, the `errors` package is using the [go-errors](https://github.com/go-errors/errors) library, but this may
change in the future, so the rest of the code should not depend on `go-errors` directly.

Here is how the `errors` package should be used:

1. Any time you want to create your own error, create a custom type for it, and when instantiating that type, wrap it
   with a call to `errors.WithStackTrace`. That way, any time you call a method defined in the Terragrunt code, you
   know the error it returns already has a stacktrace and you don't have to wrap it yourself.
1. Any time you get back an error object from a function built into Go or a 3rd party library, immediately wrap it with
   `errors.WithStackTrace`. This gives us a stacktrace as close to the source as possible.
1. If you need to get back the underlying error, you can use the `errors.IsError` and `errors.Unwrap` functions.

#### Releasing new versions

To release a new version, just go to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases) and
create a new release. The CircleCI job for this repo has been configured to:

1. Automatically detect new tags.
1. Build binaries for every OS using that tag as a version number.
1. Upload the binaries to the release in GitHub.

See `circle.yml` and `_ci/build-and-push-release-asset.sh` for details.

#### Other designs to consider

* Instead of using KMS, it may be possible to use [git-crypt](https://github.com/AGWA/git-crypt). It has the advantage
  of being relatively transparent to the user, battle-tested, and I believe it even allows you to use the native
  `git diff` to compare encrypted files. However, it has a showstopper issue: depending on what Git client you're
  using, it's possible to accidentally commit unencrypted data. This happens with the [GitHub for Mac
  client](https://github.com/AGWA/git-crypt/issues/33), but it sounds like any misconfigured client could lead to this
  condition.
* Consider using [SimpleDB](https://aws.amazon.com/simpledb/) as another possible locking mechanism.

## TODO

* Add a check that all local changes have been committed before running `terraform apply`.
* Consider embedding the Terraform Go code within Terragrunt instead of calling out to it.
* Add a `show-lock` command.
* Add a command to automatically set up best-practices remote state storage in a versioned, encrypted, S3 bucket.
* Add a command to list the different versions of state available in a versioned S3 bucket and to diff any two state
  files.