**Terragrunt is not yet compatiable with Terraform 0.9.x, but we're working on it. See [#158](https://github.com/gruntwork-io/terragrunt/issues/158) for the latest status.**

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
1. **Managing multiple modules**: Terragrunt has tools that make it easier to work with multiple Terraform folders,
   environments, and state files. 

Other types of locking mechanisms and automation for more best practices may be added in the future. 

## Motivation

When you use Terraform to provision infrastructure, it records the state of your infrastructure in [state 
files](https://www.terraform.io/docs/state/). In order to make changes to your infrastructure, everyone on your
team needs access to these state files. You could check the files into version control (not a great idea, as the state
files may contain secrets) or use a supported [remote state 
backend](https://www.terraform.io/docs/state/remote/index.html) to store the state files in a shared location such as 
[S3](https://www.terraform.io/docs/state/remote/s3.html), 
[Consul](https://www.terraform.io/docs/state/remote/consul.html), 
or [etcd](https://www.terraform.io/docs/state/remote/etcd.html). All of these options have three problems:

1. They do not provide *locking*. If two team members run `terraform apply` on the same state files at the same
   time, they may overwrite each other's changes. The official solution to this problem is to use [Hashicorp's
   Atlas](https://www.hashicorp.com/atlas.html), but that can be a fairly expensive option, and it requires you to use
   a SaaS platform for all Terraform operations.
1. They are error prone. Very often, you do a fresh checkout of a bunch of Terraform configurations from version control,
   forget to enable remote state storage before applying them, and end up creating a bunch of duplicate resources.
   Sometimes you do remember to enable remote state storage, but you use the wrong configuration (e.g. the wrong S3
   bucket name or key) and you end up overwriting the state for a totally different set of configurations.
1. If you define all of your environments (stage, prod) and components (database, app server) in one set of `.tf` files
   (and therefore one state file), then a mistake anywhere can cause problems everywhere. To isolate different 
   environments and components, you need to define your Terraform code in multiple different folders (see [How to 
   manage Terraform state](https://blog.gruntwork.io/how-to-manage-terraform-state-28f5697e68fa)), but this makes
   it harder to manage state and quickly spin up and tear down environments.

The goal of Terragrunt is to take Terraform, which is a fantastic tool, and make it even better for teams by providing
a simple, free locking mechanism, and enforcing best practices around CLI usage and state management.

## Install

1. Install [Terraform](https://www.terraform.io/), and let Terragrunt know where to find it using one of the following options:

    * Place `terraform` in a directory on your PATH.

       **Caution**: this makes it easy to accidentally invoke Terraform directly from the command line (thus bypassing the protections offered by Terragrunt).

    * Specify the full path to the Terraform binary in the environment variable `TERRAGRUNT_TFPATH`.

    * Specify the full path to the Terraform binary in `--terragrunt-tfpath` each time you run Terragrunt (see [CLI Options](#cli-options)).

1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases), downloading
   the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.

## Quick start

Go into a folder with your Terraform configurations (`.tf` files) and create a `terraform.tfvars` file with the 
following contents:

```hcl
terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "my-app"
    }
  }
  
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "my-bucket"
      key = "terraform.tfstate"
      region = "us-east-1"
    }
  }
}
```
 
By default, Terragrunt reads all of its configuration from the `terragrunt = { ... }` block in your `terraform.tfvars` 
file; Terraform also uses `terraform.tfvars` as a place where you can set values for your 
[variables](https://www.terraform.io/intro/getting-started/variables.html#from-a-file), but so long as your Terraform
code doesn't define any variables named `terragrunt`, Terraform will safely ignore this value. 

The `terraform.tfvars` file above tells Terragrunt to use [DynamoDB for locking](#locking-using-dynamodb) and to 
[automatically manage remote state](#managing-remote-state) for using the 
[S3 backend](https://www.terraform.io/docs/state/remote/s3.html). Once you check this `terraform.tfvars` file into 
source control, everyone on your team can use `terragrunt` to run all the standard `terraform` commands:

```bash
terragrunt get
terragrunt plan
terragrunt apply
terragrunt output
terragrunt destroy
```

Terragrunt forwards almost all commands, arguments, and options directly to Terraform, using whatever version of
Terraform you already have installed. However, before running Terraform, Terragrunt will ensure your remote state is
configured according to the settings in the `terraform.tfvars` file. Moreover, for the `apply`, `refresh`, and 
`destroy` commands, Terragrunt will first try to acquire a lock using [DynamoDB](#locking-using-dynamodb):

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
which are all the primitives we need for a basic distributed lock system. It's also part of [AWS's free
tier](https://aws.amazon.com/dynamodb/pricing/), and given the tiny amount of data we are working with and the 
relatively small number of times per day you're likely to run Terraform, it _should_ be a free option for teams already
using AWS. We take no responsibility for any charges you may incur.

#### DynamoDB locking prerequisites

To use DynamoDB for locking, you must:

1. Set your AWS credentials in the environment using one of the following options:
    1. Set your credentials as the environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` (and also `AWS_SESSION_TOKEN` if using [STS temporary credentials](http://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp.html)).
    1. Specify the AWS profile to use using the environment variable `AWS_PROFILE`.
    1. Specify the AWS profile to use using the `aws_profile` key in `terraform.tfvars` (see below).
    1. Run `aws configure` and fill in the details it asks for.
    1. Run Terragrunt on an EC2 instance with an IAM Role.
1. Your AWS user must have an [IAM 
   policy](http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/access-control-identity-based.html) 
   granting all DynamoDB actions (`dynamodb:*`) on the table `terragrunt_locks` (see the
   [DynamoDB locking configuration](#dynamodb-locking-configuration) for how to configure this table name).

   Here is an example IAM policy that grants the necessary permissions on the `terragrunt_locks` table in region 
   `us-west-2` for an account with account id `1234567890`:

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
 
For DynamoDB locking, Terragrunt supports the following settings in `terraform.tfvars`:

```hcl
terragrunt = {
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "my-app"
      aws_region = "us-east-1"
      table_name = "terragrunt_locks"
      max_lock_retries = 360
      aws_profile = "production"    
    }
  }
}
```

* `state_file_id`: (Required) A unique id for the state file for these Terraform configurations. Many teams have more 
  than one set of Terraform configurations, and therefore more than one state file, so this setting is used to 
  disambiguate locks for one state file from another.
* `aws_region`: (Optional) The AWS region to use. Default: `us-east-1`.
* `table_name`: (Optional) The name of the table in DynamoDB to use to store lock information. Default:
  `terragrunt_locks`.
* `max_lock_retries`: (Optional) The maximum number of times to retry acquiring a lock. Terragrunt waits 10 seconds
  between retries. Default: 360 retries (one hour).
* `aws_profile`: (Optional) The AWS login profile to use.  

#### How DynamoDB locking works

When you run `terragrunt apply` or `terragrunt destroy`, Terragrunt does the following:

1. Create the `terragrunt_locks` table if it doesn't already exist.
1. Try to write an item to the `terragrunt_locks` table with `StateFileId` equal to the `state_file_id` specified in 
   your `terraform.tfvars` file. This item will include useful metadata about the lock, such as who created it (e.g. 
   your username) and when. 
1. Note that the write is a conditional write that will fail if an item with the same `state_file_id` already exists.
    1. If the write succeeds, it means we have a lock!
    1. If the write does not succeed, it means someone else has a lock. Keep retrying every 10 seconds until we get a
       lock.
1. Run `terraform apply` or `terraform destroy`.
1. When Terraform is done, delete the item from the `terragrunt_locks` table to release the lock.
 
## Acquiring a long-term lock
 
Occasionally, you may want to lock a set of Terraform files and not allow further changes, perhaps during maintenance 
work or as a precaution for configurations that rarely change. To do that, you can use the `acquire-lock` command:
 
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

For remote state management, Terragrunt supports the following settings in `terraform.tfvars`:

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      key1 = "value1"
      key2 = "value2"
      key3 = "value3"
    }
  }
}
```

* `backend`: (Required) The name of the remote state backend to use (e.g. s3, consul).
* `config`: (Optional) A map of additional key/value pairs to pass to the backend. Each backend requires
  different key/value pairs, so consult the [Terraform remote state docs](https://www.terraform.io/docs/state/remote/)
  for details.
  
  > Note: Terragrunt will use the value provided in the `profile` key to configure the AWS SDK when using the S3 backend.

## Managing multiple Terraform configurations

With Terraform, it can be a good idea to store your configurations in separate folders (and therefore, separate state 
files) to provide isolation between different environments,such as stage and prod, and different components, such as a 
database and an app cluster (for more info, see [How to Manage Terraform 
State](https://blog.gruntwork.io/how-to-manage-terraform-state-28f5697e68fa)). That means you will need a 
`terraform.tfvars` file in each folder:

```
my-terraform-repo
  └ qa
      └ my-app
          └ main.tf
          └ terraform.tfvars
  └ stage
      └ my-app
          └ main.tf
          └ terraform.tfvars
  └ prod
      └ my-app
          └ main.tf
          └ terraform.tfvars
```

Most of these `terraform.tfvars` files will have almost the same content. For example, `qa/my-app/terraform.tfvars` may 
look like this:

```hcl
terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "qa/my-app"
    }
  }
  
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "my-bucket"
      key = "qa/my-app/terraform.tfstate"
      region = "us-east-1"
    }
  }
}
```

And `stage/my-app/terraform.tfvars` may look like this:

```hcl
terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "stage/my-app"
    }
  }
  
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "my-bucket"
      key = "stage/my-app/terraform.tfstate"
      region = "us-east-1"
    }
  }
}
```

Note how most of the content is copy/pasted, except for the `state_file_id` and `key` parameters, which match the path
of the `terraform.tfvars` file itself. How do you avoid having to manually maintain the contents of all of these 
similar-looking `terraform.tfvars` files? Also, if you want to spin up an entire environment (e.g. `stage`, `prod`), 
how do you do it without having to manually run `terragrunt apply` in each of the Terraform folders within that 
environment?

The solution is to use the following features of Terragrunt:

* Includes
* Find parent helper
* Relative path helper
* Overriding included settings
* The `apply-all`, `destroy-all`, and `output-all` commands
* Dependencies between modules

### Includes

One `terraform.tfvars` file can automatically "include" the contents of another `terraform.tfvars` file using the 
`include` block. For example, imagine you have the following file layout:

```
my-terraform-repo
  └ terraform.tfvars
  └ qa
      └ my-app
          └ main.tf
          └ terraform.tfvars
  └ stage
      └ my-app
          └ main.tf
          └ terraform.tfvars
  └ prod
      └ my-app
          └ main.tf
          └ terraform.tfvars
```

The `terraform.tfvars` file in the root folder defines the typical `lock` and `remote_state` settings. The 
`terraform.tfvars`  files in all the subfolders (e.g. `qa/my-app/terraform.tfvars`) can automatically include all the 
settings from a parent file using the `include` block:

```hcl
terragrunt = {
  include {
    path = "../../terraform.tfvars"
  }
}
```

When you run Terragrunt in the `qa/my-app` folder, it will see the `include` block in the `qa/my-app/terraform.tfvars` 
file and realize that it should load the contents of the root `terraform.tfvars` file instead. It's almost as if you had 
copy/pasted the contents of the root `terraform.tfvars` file into `qa/my-app/terraform.tfvars`, but much easier to 
maintain! Note that only the `terragrunt` section in this parent file is inserted: anything else in the file (e.g.,
variables) will not be placed into (in this case) `qa/my-app/terraform.tfvars`.

**Note**: only one level of includes is allowed. If `root/qa/my-app/terraform.tfvars` includes `root/terraform.tfvars`, 
then `root/terraform.tfvars` may NOT specify an `include` block.

There are a few problems with the simple approach above, so read on before using it!

1. Having to manually manage the file paths to the included `terraform.tfvars` file is tedious and error prone. To solve 
   this problem, you can use the `find_in_parent_folders()` helper. 
1. If the included `terraform.tfvars` file hard-codes the `state_file_id` and `key` settings, then every child that 
   includes it would end up using the same lock and write state to the same location. To avoid this problem, you can 
   use the `path_relative_to_include()` helper.
1. Some of the child `terraform.tfvars` files may want to override the settings they include. To do this, see the 
   section on overriding included settings.

Each of these items is discussed next.

### find_in_parent_folders helper

Terragrunt supports the use of a few helper functions in the `terraform.tfvars` file using the same syntax as 
Terraform: `${some_function()}`. Note that these helper functions are *only* evaluated by Terragrunt and not Terraform,
so they won't work outside of the `terragrunt = { ... }` block in the `terraform.tfvars` file.


One of the supported helper functions is `find_in_parent_folders()`, which returns the path to the first 
`terraform.tfvars` file it finds in the parent folders above the current `terraform.tfvars` file. 

Example:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

If you ran this in `qa/my-app/terraform.tfvars`, this would automatically set `path` to `../../terraform.tfvars`. You 
will almost always want to use this function, as it allows you to copy/paste the same `terraform.tfvars` file to all 
child folders with no changes.

`find_in_parent_folders()` will search up the directory tree until it hits the root folder of your file system, and if
no `terraform.tfvars` file is found, Terragrunt will exit with an error.

### path_relative_to_include helper

Another helper function supported by Terragrunt is `path_relative_to_include()`, which returns the relative path between 
the current `terraform.tfvars` file and the path specified in its `include` block. For example, in the root 
`terraform.tfvars` file, you could do the following:
 
```hcl
terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "${path_relative_to_include()}"
    }
  }
  
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "my-bucket"
      key = "${path_relative_to_include()}/terraform.tfstate"
      region = "us-east-1"
    }
  }
}
``` 

Each child `terraform.tfvars` file that references the configuration above in its `include` block will get a unique 
path for its `state_file_id` and `key` settings. For example, in `qa/my-app/terraform.tfvars`, the `state_file_id` will 
resolve to `qa/my-app` and the `key` will resolve to `qa/my-app/terraform.tfstate`.  

You will almost always want to use this helper too. The only time you may want to specify the `state_file_id` or `key` 
manually is if you moved a child folder. In that case, to ensure it can reuse its old state and lock, you may want to 
hard-code the `state_file_id` and `key` to the old file path. However, a safer approach would be to move the state 
files themselves to match the new location of the child folder, as that makes things more consistent!

### Overriding included settings

Any settings in the child `terraform.tfvars` file will override the settings pulled in via an `include`. For example, 
imagine if `qa/my-app/terraform.tfvars` had the following contents:
 
```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
  
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "some-other-bucket"
      key = "/foo/bar/terraform.tfstate"
      region = "us-west-2"
    }
  }
}
``` 

The result is that when you run `terragrunt` commands in the `qa/my-app` folder, you get the `lock` settings from the 
parent, but the `remote_state` settings of the child.

### Environment variables replacement

You can read in environment variables in the `terragrunt = { ... }` portion of your `terraform.tfvars` file using the 
`get_env()` helper function:

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "${get_env("ENVIRONMENT_VARIABLE_NAME", "development")}-bucket"
      key = "/foo/bar/terraform.tfstate"
      region = "us-west-2"
    }
  }
}
```

This function takes two parameters: `ENVIRONMENT_VARIABLE_NAME` and `default`. When parsing the file, Terragrunt 
will evaluate the environment variable `ENVIRONMENT_VARIABLE_NAME` and replace with the registered value. If there is 
no environment variable with that name or is empty, it will use the one registered in the `default`. The default value 
is mandatory but can be empty (e.g. `${get_env("ENVIRONMENT_VARIABLE_NAME", "")}`).

If there is no environment variable with that name registered in the system, the configuration file would be evaluated 
to:

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "development-bucket"
      key = "/foo/bar/terraform.tfstate"
      region = "us-west-2"
    }
  }
}
```

But if the variable is set:

```bash
ENVIRONMENT_VARIABLE="value" terragrunt
```

Then the previous example would evaluate to:

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "value-bucket"
      key = "/foo/bar/terraform.tfstate"
      region = "us-west-2"
    }
  }
}
```

Terraform itself also supports loading variables via the environment. It is possible to use the same variables by 
correctly using the terraform prefix `TF_VAR_`.

```bash
TF_VAR_variable="value" terragrunt apply
```

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "${get_env("TF_VAR_variable", "value")}-bucket"
      key = "/foo/bar/terraform.tfstate"
      region = "us-west-2"
    }
  }
}
```

### Passing extra command line arguments to Terraform

Sometimes you may need to pass extra arguments to Terraform on each run. For example if you have a separate file with
secret variables you may use extra_arguments option in terraform section of Terragrunt configuration to do it
automatically.

Each set of arguments will be appended only if current Terraform command is in `commands` list. If more than one set is
applicable, they will be added in the order of of appearance in config.

Sample config:

``` hcl
terragrunt = {
  terraform {
    extra_arguments "secrets" {
      arguments = [
        "-var-file=terraform.tfvars",
        "-var-file=terraform-secret.tfvars"
      ]
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
    }

    extra_arguments "json_output" {
      arguments = [
        "-json"
      ]
      commands = [
        "output"
      ]
    }

    extra_arguments "fmt_diff" {
      arguments = [
        "-diff=true"
      ]
      commands = [
        "fmt"
      ]
    }

  }
}
```

### The apply-all, destroy-all, and output-all commands

Let's say you have a single environment (e.g. `stage` or `prod`) that has a number of Terraform modules within it:

```
my-terraform-repo
  └ terraform.tfvars
  └ stage
      └ frontend-app
          └ main.tf
          └ terraform.tfvars
      └ backend-app
          └ main.tf
          └ terraform.tfvars
      └ search-app
          └ main.tf
          └ terraform.tfvars
      └ mysql
          └ main.tf
          └ terraform.tfvars
      └ redis
          └ main.tf
          └ terraform.tfvars
      └ vpc
          └ main.tf
          └ terraform.tfvars
```

There is one module to deploy a frontend-app, another to deploy a backend-app, another for the MySQL database, and so 
on. To deploy such an environment, you'd have to manually run `terragrunt apply` in each of the subfolders. How do you
avoid this tedious and time-consuming process?

The answer is that you can use the `apply-all` command:
 
```
cd my-terraform-repo/stage
terragrunt apply-all
```

When you run this command, Terragrunt will find all `terraform.tfvars` files in the subfolders of the current working 
directory that contain `terragrunt = { ... }` blocks, and run `terragrunt apply` in each one concurrently.

Similarly, to undeploy all the Terraform modules, you can use the `destroy-all` command:

```
cd my-terraform-repo/stage
terragrunt destroy-all
```

Finally, to see the currently applied outputs of all of the subfolders, you can use the `output-all` command:

```
cd my-terraform-repo/stage
terragrunt output-all
```

Of course, if your modules have dependencies between them—for example, you can't deploy the backend-app until the MySQL
database is deployed—you'll need to express those dependencines in your `terraform.tfvars` config as explained in the 
next section.

### Dependencies between modules

Consider the following file structure for the `stage` environment:

```
my-terraform-repo
  └ terraform.tfvars
  └ stage
      └ frontend-app
          └ main.tf
          └ terraform.tfvars
      └ backend-app
          └ main.tf
          └ terraform.tfvars
      └ search-app
          └ main.tf
          └ terraform.tfvars
      └ mysql
          └ main.tf
          └ terraform.tfvars
      └ redis
          └ main.tf
          └ terraform.tfvars
      └ vpc
          └ main.tf
          └ terraform.tfvars
```

Let's assume you have the following dependencies between Terraform modules:

* Every module depends on the VPC being deployed
* The backend-app depends on the MySQL database and Redis
* The frontend-app and search-app depend on the backend-app

You can express these dependencies in your `terraform.tfvars` config files using the `dependencies` block. For example, 
in `stage/backend-app/terraform.tfvars` you would specify:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
  
  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

Similarly, in `stage/frontend-app/terraform.tfvars`, you would specify:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
  
  dependencies {
    paths = ["../vpc", "../backend-app"]
  }
}
```

Once you've specified the depenedencies in each `terraform.tfvars` file, when you run the `terragrunt apply-all` and 
`terragrunt destroy-all`, Terragrunt will ensure that the dependencies are applied or destroyed, respectively, in the
correct order. For the example at the start of this section, the order for the `apply-all` command would be:

1. Deploy the VPC
1. Deploy MySQL and Redis in parallel
1. Deploy the backend-app
1. Deploy the frontend-app and search-app in parallel

If any of the modules fail to deploy, then Terragrunt will not attempt to deploy the modules that depend on them. Once
you've fixed the error, it's usually safe to re-run the `apply-all` or `destroy-all` command again, since it'll be a noop
for the modules that already deployed successfully, and should only affect the ones that had an error the last time
around.

## Remote Terraform configurations

A common problem with Terraform is figuring out how to minimize copy/paste between environments (i.e. stage, prod). For
example, consider the following file structure:

```
infrastructure-live
  └ stage
    └ frontend-app
        └ main.tf
        └ vars.tf
        └ outputs.tf
    └ backend-app
    └ mysql
    └ vpc
  └ prod
    └ frontend-app
        └ main.tf
        └ vars.tf
        └ outputs.tf
    └ backend-app
    └ mysql
    └ vpc
```

For each environment, you have to copy/paste `main.tf`, `vars.tf`, and `outputs.tf` for each component (e.g. 
frontend-app, backend-app, vpc, etc). As the number of components and environments grows, having to maintain more and 
more code can become error prone. You can significantly reduce the amount of copy paste using [Terraform 
modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d), but even 
the code to instantiate the module and set up input variables, output variables, providers, and remote state can still
create a lot of maintenance overhead.  

To solve this problem, Terragrunt has the ability to download Terraform configurations. How does that help? Well, 
imagine you defined the Terraform code for all of your infrastructure in a single repo, called, for example, 
infrastructure-modules:

```
infrastructure-modules
  └ frontend-app
      └ main.tf
      └ vars.tf
      └ outputs.tf
  └ backend-app
  └ mysql
  └ vpc
```

This repo contains typical Terraform code, with one difference: anything in your code that should be different between 
environments should be exposed as an input variable. For example, the frontend-app might expose a variable called
`instance_count` to determine how many instances to run and `instance_type` to determine what kind of server to deploy,
as you may want to run smaller/fewer servers in staging than in prod to save money.

In a separate repo, called, for example, infrastructure-live, you define the code for all of your environments, which
now consists of just one `.tfvars` file per component (e.g. `frontend-app.tfvars`, `backend-app.tfvars`, etc). This 
gives you the following file layout:   
 
```
infrastructure-live
  └ stage
    └ frontend-app
      └ terraform.tfvars
    └ backend-app
    └ mysql
    └ vpc
  └ prod
    └ frontend-app
      └ terraform.tfvars
    └ backend-app
    └ mysql
    └ vpc
```

This 
file defines a `terragrunt = { ... }` block to configure Terragrunt, . 

Notice how there are no Terraform configurations (`.tf` files) in any of the folders. Instead, each `.tfvars` file 
specifies a `terraform { ... }` block that specifies from where to download the Terraform code, as well as the 
environment-specific values for the input variables in that Terraform code. For example, 
`stage/frontend-app/terraform.tfvars` may look like this:
   
```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/bar.git//frontend-app?ref=v0.0.3"
  }
}

instance_count = 3
instance_type = "t2.micro"
```

*(Note: the double slash (`//`) is intentional and required. It's part of Terraform's Git syntax for [module 
sources](https://www.terraform.io/docs/modules/sources.html).)

And `prod/frontend-app/terraform.tfvars` may look like this:
   
```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/bar.git//frontend-app?ref=v0.0.1"
  }
}

instance_count = 10
instance_type = "m2.large"
```

Notice how the two `terraform.tfvars` files set the `source` URL to the same `frontend-app` module, but at different 
versions (i.e. `stage` is testing out a newer version of the module). They also set the parameters for the 
`frontend-app` module to different values that are appropriate for the environment: smaller/fewer servers in `stage` 
to save money, larger/more instances in `prod` for scalability and high availability.  

When you run Terragrunt and it finds a `terraform` block, it will:
 
1. Download the configurations specified via the `source` parameter into a temporary folder. This downloading is done 
   by using the [terraform init command](https://www.terraform.io/docs/commands/init.html), so the `source` parameter 
   supports the exact same syntax as the [module source](https://www.terraform.io/docs/modules/sources.html) parameter, 
   including local file paths, Git URLs, and Git URLs with `ref` parameters (useful for checking out a specific tag, 
   commit, or branch of Git repo). Terragrunt will download all the code in the repo (i.e. the part before the 
   double-slash `//`) so that relative paths work correctly between modules in that repo. 
1. Copy all files from the current working directory into the temporary folder. This way, Terraform will automatically
   read in the variables defined in the `terraform.tfvars` file.
1. Execute whatever Terraform command you specified in that temporary folder. **Note**: if you are passing any file
   paths (other than paths to files in the current working directory) to Terraform via command-line options, those 
   paths must be absolute paths since we will be running Terraform from the temporary folder!

With new approach, copy/paste between environments is minimized. The `.tfvars` files contain solely the variables 
that are different between environments. To create a new environment, you copy an old one and update just the 
environment-specific values in the `.tfvars` files, which is about as close to the "essential complexity" of the 
problem as you can get.

Just as importantly, since the Terraform code is now defined in a single repo, you can version it (e.g., using Git
tags and referencing them using the `ref` parameter in the `source` URL, as in the 
`stage/frontend-app/terraform.tfvars` and `prod/frontend-app/terraform.tfvars` examples above), and promote a single, 
immutable version through each environment (e.g., qa -> stage -> prod). This idea is inspired by Kief Morris' blog 
post [Using Pipelines to Manage Environments with Infrastructure as 
Code](https://medium.com/@kief/https-medium-com-kief-using-pipelines-to-manage-environments-with-infrastructure-as-code-b37285a1cbf5).

Note that you can also use the `--terragrunt-source` command-line option or the `TERRAGRUNT_SOURCE` environment variable
to override the `source` parameter. This is useful to point Terragrunt at a local checkout of your code so you can do 
rapid, iterative, make-a-change-and-rerun development:
   
```
cd infrastructure-live/stage/frontend-app
terragrunt apply --terragrunt-source ../../../infrastructure-modules//frontend-app
```
   
*(Note: the double slash (`//`) here too is intentional and required. Terragrunt downloads all the code in the folder 
before the double-slash into the temporary folder so that relative paths between modules work correctly.)*   

## CLI Options

Terragrunt forwards all arguments and options to Terraform. The only exceptions are the options that start with the
prefix `--terragrunt-`. The currently available options are:

* `--terragrunt-config`: A custom path to the `terraform.tfvars` file. May also be specified via the `TERRAGRUNT_CONFIG`
  environment variable. The default path is `terraform.tfvars` in the current directory (see 
  [Terragrunt config files](#terragrnt-config-files) for a slightly more nuanced explanation).
* `--terragrunt-tfpath`: A custom path to the Terraform binary. May also be specified via the `TERRAGRUNT_TFPATH`
  environment variable. The default is `terraform` in a directory on your PATH.
* `--terragrunt-non-interactive`: Don't show interactive user prompts. This will default the answer for all prompts to 
  'yes'. Useful if you need to run Terragrunt in an automated setting (e.g. from a script).  
* `--terragrunt-working-dir`: Set the directory where Terragrunt should execute the `terraform` command. Default is the
  current working directory. Note that for the `apply-all` and `destroy-all` directories, this parameter has a different 
  meaning: Terragrunt will apply or destroy all the Terraform modules in the subfolders of the 
  `terragrunt-working-dir`, running `terraform` in the root of each module it finds.
* `--terragrunt-source`: Download Terraform configurations from the specified source into a temporary folder, and run 
  Terraform in that temporary folder. May also be specified via the `TERRAGRUNT_SOURCE` environment variable. The 
  source should use the same syntax as the [Terraform module source](https://www.terraform.io/docs/modules/sources.html) 
  parameter.  
* `--terragrunt-source-update`: Delete the contents of the temporary folder before downloading Terraform source code
  into it.

## Terragrunt config files

The current version of Terragrunt expects configuration to be defined in a `terraform.tfvars` file. Previous
versions defined the config in a `.terragrunt` file. **The `.terragrunt` format is now deprecated**!

For backwards compatibility, Terragrunt will continue to support the `.terragrunt` file format for a short period of 
time. Check out the next section for how this works. Note that you will get a warning in your logs every time you run 
Terragrunt with a `.terragrunt` file, and we will eventually stop supporting this older format, so we recommend 
migrating to the `terraform.tfvars` format ASAP!

### Config file search paths

Terragrunt figures out the path to its config file according to the following rules:

1. The value of the `--terragrunt-config` command-line option, if specified.
1. The value of the `TERRAGRUNT_CONFIG` environment variable, if defined.
1. A `.terragrunt` file in the current working directory, if it exists.
1. A `terraform.tfvars` file in the current working directory, if it exists.
1. If none of these are found, exit with an error.

The `--terragrunt-config` parameter is only used by Terragrunt and has no effect on which variable files are loaded by Terraform. Terraform will automatically read variables from a file named terraform.tfvars, but if you want it to read variables from some other .tfvars file, you must pass it in using the `--var-file` argument:

```bash
terragrunt plan --terragrunt-config example.tfvars --var-file example.tfvars
```

### Migrating from .terragrunt to terraform.tfvars

The configuration in a `.terragrunt` file is identical to that of the `terraform.tfvars` file, except the 
`terraform.tfvars` file requires you to wrap that configuration in a `terragrunt = { ... }` block. 

For example, if this is your `.terragrunt` file:

```hcl
include {
  path = "${find_in_parent_folders()}"
}

dependencies {
  paths = ["../vpc", "../mysql", "../redis"]
}
```

The equivalent `terraform.tfvars` file is:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
  
  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

To migrate, all you need to do is:

1. Copy all the contents of the `.terragrunt` file.
1. Paste those contents into a `terragrunt = { ... }` block in a `terraform.tfvars` file.
1. Delete the `.terragrunt` file.

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

#### Formatting

Every source file in this project should be formatted with `go fmt`. There are few helper scripts and targets in the
Makefile that can help with this (mostly taken from the [terraform repo](https://github.com/hashicorp/terraform/)):

1. `make fmtcheck`

   Checks to see if all source files are formatted. Exits 1 if there are unformatted files.
1. `make fmt`

    Formats all source files with `gofmt`. 
1. `make install-pre-commit-hook`

    Installs a git pre-commit hook that will run all of the source files through `gofmt`.
    
To ensure that your changes get properly formatted, please install the git pre-commit hook with `make install-pre-commit-hook`.
    
#### Releasing new versions

To release a new version, just go to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases) and
create a new release. The CircleCI job for this repo has been configured to:

1. Automatically detect new tags.
1. Build binaries for every OS using that tag as a version number.
1. Upload the binaries to the release in GitHub.

See `circle.yml` and `_ci/build-and-push-release-asset.sh` for details.

## License

This code is released under the MIT License. See LICENSE.txt.
