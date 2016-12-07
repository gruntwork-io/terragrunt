# Terragrunt

Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that supports locking and enforces best
practices for Terraform state:

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
a simple, free locking mechanism, and enforcing best practices around CLI usage. Check out [Add Automatic Remote State Locking and Configuration to Terraform with Terragrunt](https://blog.gruntwork.io/add-automatic-remote-state-locking-and-configuration-to-terraform-with-terragrunt-656a57565a4d) for more info.

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
lock = {
  backend = "dynamodb"
  config {
    state_file_id = "my-app"
  }
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state = {
  backend = "s3"
  config {
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
configured according to the settings in the `.terragrunt` file. Moreover, for the `apply`, `refresh`, and `destroy` commands,
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
    1. Set your credentials as the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` (and also `AWS_SESSION_TOKEN` if using [STS temporary credentials](http://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp.html))
    1. Run `aws configure` and fill in the details it asks for.
    1. Run Terragrunt on an EC2 instance with an IAM Role.
1. Your AWS user must have an [IAM 
   policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html) 
   granting all DynamoDB actions (`dynamodb:*`) on the table `terragrunt_locks` (see the
   [DynamoDB locking configuration](#dynamodb-locking-configuration) for how to configure this table name).
   
   Here is an example IAM policy that grants the necessary permissions on the `terragrunt_locks` table in region `us-west-2` for
   an account with account id `1234567890`:

    ```json
    {
      "Version": "2012-10-17",
      "Statement": [
          {
            "Sid": "ReadWriteToDynamoDB",
            "Effect": "Allow",
            "Action": "dynamodb:*",
            "Resource": "arn:aws:dynamodb:us-west-2:1234567890:table/terragrunt_locks"
          }
      ]
    }
    ```

#### DynamoDB locking configuration
 
For DynamoDB locking, Terragrunt supports the following settings in `.terragrunt`:

```hcl
lock = {
  backend = "dynamodb"
  config {
    state_file_id = "my-app"
    aws_region = "us-east-1"
    table_name = "terragrunt_locks"
    max_lock_retries = 360
  }
}
```

* `state_file_id`: (Required) A unique id for the state file for these Terraform templates. Many teams have more than
  one set of templates, and therefore more than one state file, so this setting is used to disambiguate locks for one 
  state file from another.
* `aws_region`: (Optional) The AWS region to use. Default: `us-east-1`.
* `table_name`: (Optional) The name of the table in DynamoDB to use to store lock information. Default:
  `terragrunt_locks`.
* `max_lock_retries`: (Optional) The maximum number of times to retry acquiring a lock. Terragrunt waits 10 seconds
  between retries. Default: 360 retries (one hour).

#### How DynamoDB locking works

When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Create the `terragrunt_locks` table if it doesn't already exist.
1. Try to write an item to the `terragrunt_locks` table with `StateFileId` equal to the `state_file_id` specified in your
   `.terragrunt` file. This item will include useful metadata about the lock, such as who created it (e.g. your 
   username) and when. 
1. Note that the write is a conditional write that will fail if an item with the same `state_file_id` already exists.
    1. If the write succeeds, it means we have a lock!
    1. If the write does not succeed, it means someone else has a lock. Keep retrying every 10 seconds until we get a
       lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the item from the `terragrunt_locks` table to release the lock.
 
## Acquiring a long-term lock
 
Occasionally, you may want to lock a set of Terraform files and not allow further changes, perhaps during maintenance 
work or as a precaution for templates that rarely change. To do that, you can use the `acquire-lock` command:
 
```
terragrunt acquire-lock
Are you sure you want to acquire a long-term lock? (y/n): y
```
 
See the next section for how to release this lock. 
 
## Manually releasing a lock

You can use the `release-lock` command to manually release a lock. This is useful if you used the `acquire-lock` 
command to create a long-term lock or if Terragrunt shut down before it released a lock (e.g. because of `CTRL+C` or a 
crash).

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
remote_state = {
  backend = "s3"
  config {
    key1 = "value1"
    key2 = "value2"
    key3 = "value3"
  }
}
```

* `backend`: (Required) The name of the remote state backend to use (e.g. s3, consul).
* `config`: (Optional) A map of additional key/value pairs to pass to the backend. Each backend requires
  different key/value pairs, so consult the [Terraform remote state docs](https://www.terraform.io/docs/state/remote/)
  for details.

## Managing multiple .terragrunt files

With Terraform, it can be a good idea to store your templates in separate folders (and therefore, separate state files)
to provide isolation between different environments,such as stage and prod, and different components, such as a 
database and an app cluster (for more info, see [How to Manage Terraform 
State](https://blog.gruntwork.io/how-to-manage-terraform-state-28f5697e68fa)). That means you will need a `.terragrunt`
file in each folder:

```
my-terraform-repo
  └ qa
      └ my-app
          └ main.tf
          └ .terragrunt
  └ stage
      └ my-app
          └ main.tf
          └ .terragrunt
  └ prod
      └ my-app
          └ main.tf
          └ .terragrunt
```

Most of these `.terragrunt` files will have the almost the same content. For example, `qa/my-app/.terragrunt` may look
like this:

```hcl
# Configure Terragrunt to use DynamoDB for locking
lock = {
  backend = "dynamodb"
  config {
    state_file_id = "qa/my-app"
  }
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state = {
  backend = "s3"
  config {
    encrypt = "true"
    bucket = "my-bucket"
    key = "qa/my-app/terraform.tfstate"
    region = "us-east-1"
  }
}
```

And `stage/my-app/.terragrunt` may look like this:

```hcl
# Configure Terragrunt to use DynamoDB for locking
lock = {
  backend = "dynamodb"
  config {
    state_file_id = "stage/my-app"
  }
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state = {
  backend = "s3"
  config {
    encrypt = "true"
    bucket = "my-bucket"
    key = "stage/my-app/terraform.tfstate"
    region = "us-east-1"
  }
}
```

Note how most of the content is copy/pasted, except for the `state_file_id` and `key` parameters, which match the path
of the `.terragrunt` file itself. How do you avoid having to manually maintain the contents of all of these 
similar-looking `.terragrunt` files? Also, if you want to spin up an entire environment (e.g. `stage`, `prod`), how do
you do it without having to manually run `terragrunt apply` in each of the Terraform folders within that environment?

The solution is to use the following features of Terragrunt:

* Includes
* Find parent helper
* Relative path helper
* Overriding included settings
* The spin-up and tear-down commands
* Dependencies between modules

### Includes

One `.terragrunt` file can automatically "include" the contents of another `.terragrunt` file using the `include` 
block. For example, imagine you have the following file layout:

```
my-terraform-repo
  └ .terragrunt
  └ qa
      └ my-app
          └ main.tf
          └ .terragrunt
  └ stage
      └ my-app
          └ main.tf
          └ .terragrunt
  └ prod
      └ my-app
          └ main.tf
          └ .terragrunt
```

The `.terragrunt` file in the root folder defines the typical `lock` and `remote_state` settings. The `.terragrunt` 
files in all the subfolders (e.g. `qa/my-app/.terragrunt`) can automatically include all the settings from a parent 
file using the `include` block:

```hcl
include = {
  path = "../../.terragrunt"
}
```

When you run Terragrunt in the `qa/my-app` folder, it will see the `include` block in the `qa/my-app/.terragrunt` file
and realize that it should load the contents of the root `.terragrunt` file instead. It's almost as if you had 
copy/pasted the contents of the root `.terragrunt` file into `qa/my-app/.terragrunt`, but much easier to maintain!

**Note**: only one level of includes is allowed. If `root/qa/my-app/.terragrunt` includes `root/.terragrunt`, then 
`root/.terragrunt` may NOT specify an `include` block.

There are a few problems with the simple approach above, so read on before using it!

1. Having to manually manage the file paths to the included `.terragrunt` file is tedious and error prone. To solve 
   this problem, you can use the `find_in_parent_folders()` helper. 
1. If the included `.terragrunt` file hard-codes the `state_file_id` and `key` settings, then every child that includes
   it would end up using the same lock and write state to the same location. To avoid this problem, you can use the 
   `path_relative_to_include()` helper.
1. Some of the child `.terragrunt` files may want to override the settings they include. To do this, see the section
   on overriding included settings.

Each of these items is discussed next.

### find_in_parent_folders helper

Terragrunt supports the use of a few helper functions using the same syntax as Terraform: `${some_function()}`. One of
the supported helper functions is `find_in_parent_folders()`, which returns the path to the first `.terragrunt` file it
finds in the parent folders above the current `.terragrunt` file. 

Example:

```hcl
include = {
  path = "${find_in_parent_folders()}"
}
```

If you ran this in `qa/my-app/.terragrunt`, this would automatically set `path` to `../../.terragrunt`. You will almost
always want to use this function, as it allows you to copy/paste the same `.terragrunt` file to all child folders with
no changes.

`find_in_parent_folders()` will search up the directory tree until it hits the root folder of your file system, and if
no `.terragrunt` file is found, Terragrunt will exit with an error.

### path_relative_to_include helper

Another helper function supported by Terragrunt is `path_relative_to_include()`, which returns the relative path between 
the current `.terragrunt` file and the path specified in its `include` block. For example, in the root `.terragrunt` 
file, you could do the following:
 
```hcl
# Configure Terragrunt to use DynamoDB for locking
lock = {
  backend = "dynamodb"
  config {
    state_file_id = "${path_relative_to_include()}"
  }
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state = {
  backend = "s3"
  config {
    encrypt = "true"
    bucket = "my-bucket"
    key = "${path_relative_to_include()}/terraform.tfstate"
    region = "us-east-1"
  }
}
``` 

Each child `.terragrunt` file that references the configuration above in its `include` block will get a unique path for 
its `state_file_id` and `key` settings. For example, in `qa/my-app/.terragrunt`, the `state_file_id` will resolve to 
`qa/my-app` and the `key` will resolve to `qa/my-app/terraform.tfstate`.  

You will almost always want to use this helper too. The only time you may want to specify the `state_file_id` or `key` 
manually is if you moved a child folder. In that case, to ensure it can reuse its old state and lock, you may want to 
hard-code the `state_file_id` and `key` to the old file path. However, a safer approach would be to move the state 
files themselves to match the new location of the child folder, as that makes things more consistent!

### Overriding included settings

Any settings in the child `.terragrunt` file will override the settings pulled in via an `include`. For example, 
imagine if `qa/my-app/.terragrunt` had the following contents:
 
```hcl
include = {
  path = "${find_in_parent_folders()}"
}

remote_state = {
  backend = "s3"
  config {
    encrypt = "true"
    bucket = "some-other-bucket"
    key = "/foo/bar/terraform.tfstate"
    region = "us-west-2"
  }
}
``` 

The result is that when you run `terragrunt` commands in the `qa/my-app` folder, you get the `lock` settings from the 
parent, but the `remote_state` settings of the child. 

### The spin-up and tear-down commands

Let's say you have a single environment (e.g. `stage` or `prod`) that has a number of Terraform modules within it:

```
my-terraform-repo
  └ .terragrunt
  └ stage
      └ frontend-app
          └ main.tf
          └ .terragrunt
      └ backend-app
          └ main.tf
          └ .terragrunt
      └ search-app
          └ main.tf
          └ .terragrunt
      └ mysql
          └ main.tf
          └ .terragrunt
      └ redis
          └ main.tf
          └ .terragrunt
      └ vpc
          └ main.tf
          └ .terragrunt
```

There is one module to deploy a frontend-app, another to deploy a backend-app, another for the MySQL database, and so 
on. To deploy such an environment, you'd have to manually run `terragrunt apply` in each of the subfolders. How do you
avoid this tedious and time-consuming process?

The answer is that you can use the `spin-up` command:
 
```
cd my-terraform-repo/stage
terragrunt spin-up
```

When you run this command, Terragrunt will find all `.terragrunt` files in the subfolders of the current working 
directory, and run `terragrunt apply` in each one concurrently.  

Similarly, to undeploy all the Terraform modules, you can use the `tear-down` command:

```
cd my-terraform-repo/stage
terragrunt tear-down
```

Of course, if your modules have dependencies between them—for example, you can't deploy the backend-app until the MySQL
database is deployed—you'll need to express those dependencines in your `.terragrunt` config as explained in the next
section.

### Dependencies between modules

Consider the following file structure for the `stage` environment:

```
my-terraform-repo
  └ .terragrunt
  └ stage
      └ frontend-app
          └ main.tf
          └ .terragrunt
      └ backend-app
          └ main.tf
          └ .terragrunt
      └ search-app
          └ main.tf
          └ .terragrunt
      └ mysql
          └ main.tf
          └ .terragrunt
      └ redis
          └ main.tf
          └ .terragrunt
      └ vpc
          └ main.tf
          └ .terragrunt
```

Let's assume you have the following dependencies between Terraform modules:

* Every module depends on the VPC being deployed
* The backend-app depends on the MySQL database and Redis
* The frontend-app and search-app depend on the backend-app

You can express these dependencies in your `.terragrunt` config files using the `dependencies` block. For example, in
`stage/backend-app/.terragrunt` you would specify:

```hcl
include = {
  path = "${find_in_parent_folders()}"
}

dependencies = {
  paths = ["../vpc", "../mysql", "../redis"]
}
```

Similarly, in `stage/frontend-app/.terragrunt`, you would specify:

```hcl
include = {
  path = "${find_in_parent_folders()}"
}

dependencies = {
  paths = ["../vpc", "../backend-app"]
}
```

Once you've specified the depenedencies in each `.terragrunt` file, when you run the `terragrunt spin-up` and 
`terragrunt tear-down`, Terragrunt will ensure that the dependencies are applied or destroyed, respectively, in the
correct order. For the example at the start of this section, the order for the `spin-up` command would be:

1. Deploy the VPC
1. Deploy MySQL and Redis in parallel
1. Deploy the backend-app
1. Deploy the frontend-app and search-app in parallel

If any of the modules fail to deploy, then Terragrunt will not attempt to deploy the modules that depend on them. Once
you've fixed the error, it's usually safe to re-run the `spin-up` or `tear-down` command again, since it'll be a noop
for the modules that already deployed successfully, and should only affect the ones that had an error the last time
around.

## CLI Options

Terragrunt forwards all arguments and options to Terraform. The only exceptions are the options that start with the
prefix `--terragrunt-`. The currently available options are:

* `--terragrunt-config`: A custom path to the `.terragrunt` file. May also be specified via the `TERRAGRUNT_CONFIG`
  environment variable. The default path is `.terragrunt` in the current directory.
* `--terragrunt-non-interactive`: Don't show interactive user prompts. This will default the answer for all prompts to 
  'yes'. Useful if you need to run Terragrunt in an automated setting (e.g. from a script).  
* `--terragrunt-working-dir`: Set the directory where Terragrunt should execute the `terraform` command. Default is the
  current working directory. Note that for the `spin-up` and `tear-down` directories, this parameter has a different 
  meaning: Terragrunt will apply or destroy all the Terraform modules in the subfolders of the 
  `terragrunt-working-dir`, running `terraform` in the root of each module it finds.

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

## License

This code is released under the MIT License. See LICENSE.txt.

## TODO

* Add a check that modules have been downloaded using `terraform get`.
* Add a check that all local changes have been committed before running `terraform apply`.
* Consider implementing alternative locking mechanisms, such as using Git instead of DynamoDB.
* Consider embedding the Terraform Go code within Terragrunt instead of calling out to it.
* Add a `show-lock` command.
* Add a command to automatically set up best-practices remote state storage in a versioned, encrypted, S3 bucket.
* Add a command to list the different versions of state available in a versioned S3 bucket and to diff any two state
  files.
