# Terragrunt

Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that supports locking and enforces best
practices:

1. **Locking**: Terragrunt can use Amazon's [DynamoDB](https://aws.amazon.com/dynamodb/) as a distributed locking
   mechanism to ensure that two team members working on the same Terraform state files do not overwrite each other's
   changes. DynamoDB is part of the [AWS free tier](https://aws.amazon.com/dynamodb/pricing/), so using it as a locking
   mechanism should not cost you anything.
1. **Remote state management**: A common mistake when using Terraform is to forget to configure remote state or to
   configure it incorrectly. Terragrunt can prevent these sorts of errors by automatically configuring remote state for
   everyone on your team.

Other types of locking mechanisms and automation for more best practices may be added in the future.

## Motivation

When you use Terraform to provision infrastructure, it records the state of your infrastructure in [state 
files](https://www.terraform.io/docs/state/). In order to make changes to your infrastructure, everyone on your
team needs access to these state files. You could check the files into version control (not a great idea, as the state
files may contain secrets) or use a supported [remote state 
backend](https://www.terraform.io/docs/state/remote/index.html) to store the state files in a shared location such as 
[S3](https://www.terraform.io/docs/state/remote/s3.html), 
[Consul](https://www.terraform.io/docs/state/remote/consul.html), 
or [etcd](https://www.terraform.io/docs/state/remote/etcd.html). All of these options have two problems:

1. They do not provide *locking*. If two team members run `terraform apply` on the same state files at the same
   time, they may overwrite each other's changes. The official solution to this problem is to use [Hashicorp's
   Atlas](https://www.hashicorp.com/atlas.html), but that can be a fairly expensive option, and it requires you to use
   a SaaS platform for all Terraform operations.
1. They are error prone. Very often, you do a fresh checkout of a bunch of Terraform templates from version control,
   forget to enable remote state storage before applying them, and end up creating a bunch of duplicate resources.
   Sometimes you do remember to enable remote state storage, but you use the wrong configuration (e.g. the wrong S3
   bucket name or key) and you end up overwriting the state for a totally different set of templates.

The goal of Terragrunt is to take Terraform, which is a fantastic tool, and make it even better for teams by providing
a simple, free locking mechanism, and enforcing best practices around CLI usage.

## Install

1. Install [Terraform](https://www.terraform.io/) and make sure it is in your PATH.
1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases), downloading
   the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.

## Quick start

Go into the folder with your Terraform templates and create a `.terragrunt` file. This file uses the same
[HCL](https://github.com/hashicorp/hcl) syntax as Terraform. Here is an example `.terragrunt` file that configures
Terragrunt to use [DynamoDB for locking](#locking-using-dynamodb) and to [automatically manage remote
state](#managing-remote-state) for you using the [S3 backend](https://www.terraform.io/docs/state/remote/s3.html):

```hcl
# Configure Terragrunt to use DynamoDB for locking
dynamoDbLock = {
  stateFileId = "my-app"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
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

Once you check this `.terragrunt` file into source control, everyone on your team can use Terragrunt to run all the
standard Terraform commands:

```bash
terragrunt get
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
```

Terragrunt forwards almost all commands, arguments, and options directly to Terraform, using whatever version of
Terraform you already have installed. However, before running Terraform, Terragrunt will ensure your remote state is
configured according to the settings in the `.terragrunt` file. Moreover, for the `apply` and `destroy` commands,
Terragrunt will first try to acquire a lock using [DynamoDB](#locking-using-dynamodb):

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
 
## Cleaning up old locks

If Terragrunt is shut down before it releases a lock (e.g. via `CTRL+C` or a crash), the lock might not be deleted, and
will prevent future changes to your state files. To clean up old locks, you can use the `release-lock` command:

```
terragrunt release-lock
Are you sure you want to forcibly remove the lock for stateFileId "my-app"? (y/n): y
```

## Managing remote state

Terragrunt can automatically manage [remote state](https://www.terraform.io/docs/state/remote/) for you, preventing
manual errors such as forgetting to enable remote state or using the wrong settings.

#### Remote state management prerequisites

Terragrunt works with all backends supported by Terraform. Check out the [Terraform remote state
docs](https://www.terraform.io/docs/state/remote/) for the requirements to use a particular remote state backend.

#### Remote state management configuration

For remote state management, Terragrunt supports the following settings in `.terragrunt`:

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

* `backend`: (Required) The name of the remote state backend to use (e.g. s3, consul).
* `backendConfigs`: (Optional) A map of additional key/value pairs to pass to the backend. Each backend requires
  different key/value pairs, so consult the [Terraform remote state docs](https://www.terraform.io/docs/state/remote/)
  for details.

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

## TODO

* Add a check that modules have been downloaded using `terraform get`.
* Add a check that all local changes have been committed before running `terraform apply`.
* Consider implementing alternative locking mechanisms, such as using Git instead of DynamoDB.
* Consider embedding the Terraform Go code within Terragrunt instead of calling out to it.
* To avoid storing tfstate files on disk, consider keeping them solely in memory and feeding Terraform the file via
  `/dev/stdin`, process substitution, or one of the other options described
  [here](http://stackoverflow.com/questions/4092252/is-there-any-way-to-supply-stdin-out-instead-of-a-file-to-a-program-in-unix).
* Add a `show-lock` command.
* Use IAM username instead of the local OS username in lock metadata.