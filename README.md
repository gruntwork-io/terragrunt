# Terragrunt

Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that provides extra tools for working with 
multiple Terraform modules, remote state, and locking.




## Quick start

1. Install [Terraform](https://www.terraform.io/).

1. Install Terragrunt by going to the [Releases Page](https://github.com/gruntwork-io/terragrunt/releases), downloading
   the binary for your OS, renaming it to `terragrunt`, and adding it to your PATH.

1. Go into a folder with your Terraform configurations (`.tf` files) and create a `terraform.tfvars` file with a
   `terragrunt = { ... }` block that contains the configuration for Terragrunt (check out the [Use cases](#use-cases) 
   section for the types of configuration Terragrunt supports):
   
    ```hcl
    terragrunt = {
      # (put your Terragrunt configuration here)
    }
    ```
    
1. Now, instead of running `terraform` directly, run all the standard Terraform commands using `terragrunt`:
   
    ```bash
    terragrunt get
    terragrunt plan
    terragrunt apply
    terragrunt output
    terragrunt destroy
    ```
   
   Terragrunt forwards almost all commands, arguments, and options directly to Terraform, using whatever version of
   Terraform you already have installed. However, based on the settings in your `terraform.tfvars` file, Terragrunt can
   configure remote state, locking, extra arguments, and lots more. 
    
       
       
       


## Use cases

Terragrunt supports the following use cases:

1. [Keep your Terraform code DRY](#keep-your-terraform-code-dry)
1. [Keep your remote state configuration DRY](#keep-your-remote-state-configuration-dry)
1. [Keep your CLI flags DRY](#keep-your-cli-flags-dry)
1. [Work with multiple Terraform modules](#work-with-multiple-terraform-modules)






### Keep your Terraform code DRY

#### Motivation

Consider the following file structure, which defines three environments (prod, qa, stage) with the same infrastructure
in each one (an app, a MySQL database, and a VPC):

```
└── live
    ├── prod
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    ├── qa
    │   ├── app
    │   │   └── main.tf
    │   ├── mysql
    │   │   └── main.tf
    │   └── vpc
    │       └── main.tf
    └── stage
        ├── app
        │   └── main.tf
        ├── mysql
        │   └── main.tf
        └── vpc
            └── main.tf
```

The contents of each environment will be more or less identical, except perhaps for a few settings (e.g. the prod
environment may run bigger or more servers). As the size of the infrastructure grows, having to maintain all of this  
duplicated code between environments becomes more error prone. You can reduce the amount of copy paste using 
[Terraform modules](https://blog.gruntwork.io/how-to-create-reusable-infrastructure-with-terraform-modules-25526d65f73d), 
but even the code to instantiate a module and set up input variables, output variables, providers, and remote state 
can still create a lot of maintenance overhead.  

How can you keep your Terraform code [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) so that you only 
have to define it once, no matter how many environments you have?


#### Remote Terraform configurations

Terragrunt has the ability to download remote Terraform configurations. The idea is that you define the Terraform code 
for your infrastructure just once, in a single repo, called, for example, `modules`:

```
└── modules
    ├── app
    │   └── main.tf
    ├── mysql
    │   └── main.tf
    └── vpc
        └── main.tf
```

This repo contains typical Terraform code, with one difference: anything in your code that should be different between 
environments should be exposed as an input variable. For example, the `app` module might expose the following 
variables: 

```hcl
variable "instance_count" {
  description = "How many servers to run"
}

variable "instance_type" {
  description = "What kind of servers to run (e.g. t2.large)"
}
```

These variables allow you to run smaller/fewer servers in qa and stage to save money and larger/more servers in prod to
ensure availability and scalability.

In a separate repo, called, for example, `live`, you define the code for all of your environments, which now consists 
of just one `.tfvars` file per component (e.g. `app/terraform.tfvars`, `mysql/terraform.tfvars`, etc). This gives you 
the following file layout:   
 
```
└── live
    ├── prod
    │   ├── app
    │   │   └── terraform.tfvars
    │   ├── mysql
    │   │   └── terraform.tfvars
    │   └── vpc
    │       └── terraform.tfvars
    ├── qa
    │   ├── app
    │   │   └── terraform.tfvars
    │   ├── mysql
    │   │   └── terraform.tfvars
    │   └── vpc
    │       └── terraform.tfvars
    └── stage
        ├── app
        │   └── terraform.tfvars
        ├── mysql
        │   └── terraform.tfvars
        └── vpc
            └── terraform.tfvars
```

Notice how there are no Terraform configurations (`.tf` files) in any of the folders. Instead, each `.tfvars` file 
specifies a `terraform { ... }` block that specifies from where to download the Terraform code, as well as the 
environment-specific values for the input variables in that Terraform code. For example, 
`stage/app/terraform.tfvars` may look like this:
   
```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.3"
  }
}

instance_count = 3
instance_type = "t2.micro"
```

*(Note: the double slash (`//`) is intentional and required. It's part of Terraform's Git syntax for [module 
sources](https://www.terraform.io/docs/modules/sources.html).)*

And `prod/app/terraform.tfvars` may look like this:
   
```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//app?ref=v0.0.1"
  }
}

instance_count = 10
instance_type = "m2.large"
```

Notice how the two `terraform.tfvars` files set the `source` URL to the same `app` module, but at different 
versions (i.e. `stage` is testing out a newer version of the module). They also set the parameters for the 
`app` module to different values that are appropriate for the environment: smaller/fewer servers in `stage` 
to save money, larger/more instances in `prod` for scalability and high availability.  


#### How to use remote configurations

Once you've set up your `live` and `modules` repositories, all you need to do is run `terragrunt` commands in the 
`live` repository. For example, to deploy the `app` module in qa, you would do the following:

```
cd live/qa/app
terragrunt apply
```

When Terragrunt finds the `terraform` block with a `source` parameter in `live/qa/app/terraform.tfvars` file, it will:
 
1. Download the configurations specified via the `source` parameter into a temporary folder. This downloading is done 
   by using the [terraform init command](https://www.terraform.io/docs/commands/init.html), so the `source` parameter 
   supports the exact same syntax as the [module source](https://www.terraform.io/docs/modules/sources.html) parameter, 
   including local file paths, Git URLs, and Git URLs with `ref` parameters (useful for checking out a specific tag, 
   commit, or branch of Git repo). Terragrunt will download all the code in the repo (i.e. the part before the 
   double-slash `//`) so that relative paths work correctly between modules in that repo. 

1. Copy all files from the current working directory into the temporary folder. This way, Terraform will automatically
   read in the variables defined in the `terraform.tfvars` file.

1. Execute whatever Terraform command you specified in that temporary folder. 


#### DRY Terraform code and immutable infrastructure

With this new approach, copy/paste between environments is minimized. The `.tfvars` files contain solely the variables 
that are different between environments. To create a new environment, you copy an old one and update just the 
environment-specific values in the `.tfvars` files, which is about as close to the "essential complexity" of the 
problem as you can get.

Just as importantly, since the Terraform module code is now defined in a single repo, you can version it (e.g., using Git
tags and referencing them using the `ref` parameter in the `source` URL, as in the `stage/app/terraform.tfvars` and 
`prod/app/terraform.tfvars` examples above), and promote a single, immutable version through each environment (e.g., 
qa -> stage -> prod). This idea is inspired by Kief Morris' blog post [Using Pipelines to Manage Environments with 
Infrastructure as Code](https://medium.com/@kief/https-medium-com-kief-using-pipelines-to-manage-environments-with-infrastructure-as-code-b37285a1cbf5).


#### Working locally

If you're testing changes to a local copy of the `modules` repo, you you can use the `--terragrunt-source` command-line 
option or the `TERRAGRUNT_SOURCE` environment variable to override the `source` parameter. This is useful to point 
Terragrunt at a local checkout of your code so you can do rapid, iterative, make-a-change-and-rerun development:
   
```
cd live/stage/app
terragrunt apply --terragrunt-source ../../../modules//app
```
   
*(Note: the double slash (`//`) here too is intentional and required. Terragrunt downloads all the code in the folder 
before the double-slash into the temporary folder so that relative paths between modules work correctly.)*   


#### Important gotcha: working with relative file paths

One of the gotchas with downloading Terraform configurations is that when you run `terragrunt apply` in folder `foo`,
Terraform will actually execute in some temporary folder such as `/tmp/foo`. That means you have to be especially 
careful with relative file paths, as they will be relative to that temporary folder and not the folder where you ran
Terragrunt!

In particular:

* **Command line**: When using file paths on the command line, such as passing an extra `-var-file` argument, you 
  should use absolute paths:

    ```bash
    # Use absolute file paths on the CLI!
    terragrunt apply -var-file /foo/bar/extra.tfvars
    ```

* **Terragrunt configuration**: When using file paths directly in your Terragrunt configuration (`terraform.tfvars`), 
  such as in an `extra_arguments` block, you can't use hard-coded absolute file paths, or it won't work on your 
  teammates' computers. Therefore, you should use a relative file path with the `get_tfvars_dir()` helper:

    ```hcl
    terragrunt = {
      terraform {
        source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"
        
        extra_arguments "custom_vars" {
          commands = [
            "apply",
            "plan",
            "import",
            "push",
            "refresh"
          ]
          
          # With the get_tfvars_dir helper, you can use relative paths! 
          arguments = [
            "-var-file=${get_tfvars_dir()}/../common.tfvars",
            "-var-file=terraform.tfvars"
          ]
        }    
      }
    }
    ```

  See the [get_tfvars_dir()](#get_tfvars_dir) documentation for more details.









### Keep your remote state configuration DRY

#### Motivation

Terraform supports [remote state storage](https://www.terraform.io/docs/state/remote.html) via a variety of 
[backends](https://www.terraform.io/docs/backends) that you configure as follows:

```hcl
terraform {
  backend "s3" {
    bucket     = "my-terraform-state"
    key        = "frontend-app/terraform.tfstate"
    region     = "us-east-1"
    encrypt    = true
    lock_table = "my-lock-table"
  }
}
```

Unfortunately, the `backend` configuration does not support interpolation. This makes it hard to keep your code 
[DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) if you have multiple Terraform modules. For example,
consider the following folder structure, which uses different Terraform modules to deploy a backend app, frontend app,
MySQL database, and a VPC:

```
├── backend-app
│   └── main.tf
├── frontend-app
│   └── main.tf
├── mysql
│   └── main.tf
└── vpc
    └── main.tf
```

To use remote state with each of these modules, you would have to copy/paste the exact same `backend` configuration 
into each of the `main.tf` files. The only thing that would differ between the configurations would be the `key` 
parameter: e.g., the `key` for `mysql/main.tf` might be `mysql/terraform.tfstate` and the `key` for 
`frontend-app/main.tf` might be `frontend-app/terraform.tfstate`. 

To keep your remote state configuration DRY, you can use Terragrunt. You still have to specify the `backend` you want 
to use in each module, but instead of copying and pasting the configuration settings over and over again into each 
`main.tf` file, you can leave them blank:
 
```hcl
terraform {
  # The configuration for this backend will be filled in by Terragrunt
  backend "s3" {}
}
``` 


#### Filling in remote state settings with Terragrunt

To fill in the settings via Terragrunt, create a `terraform.tfvars` file in the root folder and in each of the 
Terraform modules:

```
├── terraform.tfvars
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

In your **root** `terraform.tfvars` file, you can define your entire remote state configuration just once in a 
`remote_state` block, as follows:

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket     = "my-terraform-state"
      key        = "${path_relative_to_include()}/terraform.tfstate"
      region     = "us-east-1"
      encrypt    = true
      lock_table = "my-lock-table"
    }
  }
}
```

The `remote_state` block supports all the same [backend types](https://www.terraform.io/docs/backends/types/index.html) 
as Terraform. The next time you run `terragrunt`, it will automatically configure all the settings in the
`remote_state.config` block, if they aren't configured already, by calling [terraform 
init](https://www.terraform.io/docs/commands/init.html).

In each of the **child** `terraform.tfvars` files, such as `mysql/terraform.tfvars`, you can tell Terragrunt to 
automatically include all the settings from the root `terraform.tfvars` file as follows:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

The `include` block tells Terragrunt to use the exact settings from the `terraform.tfvars` file specified via the 
`path` parameter. It behaves exactly as if you had copy/pasted the contents of the root `terraform.tfvars` file into 
`mysql/terraform.tfvars`, but this approach is much easier to maintain! Note that if you include any other settings
in the `terragrunt` block of a child `.tfvars` file, it will override the settings in the parent.

The `terraform.tfvars` files above use two *helper functions*: 

* `find_in_parent_folders()`: This helper returns the path to the first `terraform.tfvars` file it finds in the parent 
  folders above the current `terraform.tfvars` file. In the example above, the call to `find_in_parent_folders()` in 
  `mysql/terraform.tfvars` will return `../terraform.tfvars`. This way, you don't have to hard code the `path` 
  parameter in every module.

* `path_relative_to_include()`: This helper function returns the relative path between the current `terraform.tfvars` 
  file and the path specified in its `include` block. We typically use this in a root `terraform.tfvars` file so that 
  each Terraform child module stores its Terraform state at a different `key`. For example, the `mysql` module will 
  have its `key` parameter resolve to `mysql/terraform.tstate` and the `frontend-app` module will have its `key` 
  parameter resolve to `frontend-app/terraform.tfstate`.

See [the helper functions docs](#helper-functions) for more info.


#### Create remote state and locking resources automatically

When you run `terragrunt` with `remote_state` configuration, it will automatically create the following resources if 
they don't already exist:

* **S3 bucket**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for remote 
  state storage and the `bucket` you specify in `remote_state.config` doesn't already exist, Terragrunt will create it 
  automatically, with [versioning enabled](http://docs.aws.amazon.com/AmazonS3/latest/dev/Versioning.html). 

* **DynamoDB table**: If you are using the [S3 backend](https://www.terraform.io/docs/backends/types/s3.html) for 
  remote state storage and you specify a `lock_table` (a [DynamoDB table used for 
  locking](https://www.terraform.io/docs/backends/types/s3.html#lock_table)) in `remote_state.config`, if that table
  doesn't already exist, Terragrunt will create it automatically, including a primary key called `LockID`.

**Note**: If you specify a `profile` key in `remote_state.config`, Terragrunt will automatically use this AWS profile 
when creating the S3 bucket or DynamoDB table. 







### Keep your CLI flags DRY

#### Motivation

Sometimes you may need to pass extra CLI arguments every time you run certain `terraform` commands. For example, you 
may want to set the `lock-timeout` setting to 20 minutes for all commands that may modify remote state so that 
Terraform will keep trying to acquire a lock for up to 20 minutes if someone else already has the lock rather than 
immediately exiting with an error.

You can configure Terragrunt to pass specific CLI arguments for specific commands using an `extra_arguments` block
in your `terraform.tfvars` file:

```hcl
terragrunt = {
  terraform {
    # Force Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock 
    extra_arguments "retry_lock" {
      commands = [
        "init",
        "apply",
        "refresh",
        "import",
        "plan",
        "taint",
        "untaint"
      ]
      
      arguments = [
        "-lock-timeout=20m"
      ]    
    } 
  }
}
```

Each `extra_arguments` block includes an arbitrary name (in the example above, `retry_lock`), a list of `commands` to
which the extra arguments should be add, and the list of `arguments` to add. With the configuration above, when you 
run `terragrunt apply`, Terragrunt will call Terraform as follows:

```
> terragrunt apply

terraform apply -lock-timeout=20m
```


#### Multiple extra_arguments blocks
 
You can specify one or more `extra_arguments` blocks. The `arguments` in each block will be applied any time you call
`terragrunt` with one of the commands in the `commands` list. If more than one `extra_arguments` block matches a 
command, the arguments will be added in the order of of appearance in the configuration. For example, in addition to 
lock settings, you may also want to pass custom `-var-file` arguments to several commands:

```hcl
terragrunt = {
  terraform {
    # Force Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock 
    extra_arguments "retry_lock" {
      commands = [
        "init",
        "apply",
        "refresh",
        "import",
        "plan",
        "taint",
        "untaint"
      ]
      
      arguments = [
        "-lock-timeout=20m"
      ]    
    } 
  
    # Pass custom var files to Terraform
    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
      
      arguments = [
        "-var-file=terraform.tfvars",
        "-var-file=terraform-secret.tfvars"
      ]
    }
  }
}
```

With the configuration above, when you run `terragrunt apply`, Terragrunt will call Terraform as follows:

```
> terragrunt apply

terraform apply -lock-timeout=20m -var-file=terraform.tfvars -var-file=terraform-secret.tfvars
```


#### Handling whitespace

The list of arguments cannot include whitespaces, so if you need to pass command line arguments that include 
spaces (e.g. `-var bucket=example.bucket.name`), then each of the arguments will need to be a separate item in the
`arguments` list:

```hcl
terragrunt = {
  terraform {
    extra_arguments "bucket" {
      arguments = [
        "-var", "bucket=example.bucket.name",
      ]
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
    }
  }
}
```

With the configuration above, when you run `terragrunt apply`, Terragrunt will call Terraform as follows:

```
> terragrunt apply

terraform apply -var bucket=example.bucket.name
```






### Work with multiple Terraform modules


#### Motivation

Let's say your infrastructure is defined across multiple Terraform modules:

```
root
├── backend-app
│   └── main.tf
├── frontend-app
│   └── main.tf
├── mysql
│   └── main.tf
├── redis
│   └── main.tf
└── vpc
    └── main.tf
```

There is one module to deploy a frontend-app, another to deploy a backend-app, another for the MySQL database, and so
on. To deploy such an environment, you'd have to manually run `terraform apply` in each of the subfolder, wait for it 
to complete, and then run `terraform apply` in the next subfolder. How do you avoid this tedious and time-consuming 
process?


#### The apply-all, destroy-all, and output-all commands

To be able to deploy multiple Terraform modules in a single command, add a `terraform.tfvars` file to each module:

```
root
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
├── redis
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

Inside each `terraform.tfvars` file, add a `terragrunt = { ... }` block to identify this as a module managed by 
Terragrunt (the block can be empty or include any of the configs described in this documentation):

```hcl
terragrunt = {
  # Put your Terragrunt configuration here
}
```

Now you can go into the `root` folder and deploy all the modules within it by using the `apply-all` command:
 
```
cd root
terragrunt apply-all
```

When you run this command, Terragrunt will recursively look through all the subfolders of the current working 
directory, find all `terraform.tfvars` files with a `terragrunt = { ... }` block, and run `terragrunt apply` in each 
one concurrently.

Similarly, to undeploy all the Terraform modules, you can use the `destroy-all` command:

```
cd root
terragrunt destroy-all
```

Finally, to see the currently applied outputs of all of the subfolders, you can use the `output-all` command:

```
cd root
terragrunt output-all
```

If your modules have dependencies between them—for example, you can't deploy the backend-app until MySQL and redis are 
deployed—you'll need to express those dependencines in your Terragrunt configuation as explained in the next section.


#### Dependencies between modules

Consider the following file structure:

```
root
├── backend-app
│   ├── main.tf
│   └── terraform.tfvars
├── frontend-app
│   ├── main.tf
│   └── terraform.tfvars
├── mysql
│   ├── main.tf
│   └── terraform.tfvars
├── redis
│   ├── main.tf
│   └── terraform.tfvars
└── vpc
    ├── main.tf
    └── terraform.tfvars
```

Let's assume you have the following dependencies between Terraform modules:

* `backend-app` depends on `mysql`, `redis`, and `vpc`
* `frontend-app` depends on `backend-app` and `vpc`
* `mysql` depends on `vpc`
* `redis` depends on `vpc`
* `vpc` has no dependencies

You can express these dependencies in your `terraform.tfvars` config files using a `dependencies` block. For example, 
in `backend-app/terraform.tfvars` you would specify:

```hcl
terragrunt = {
  dependencies {
    paths = ["../vpc", "../mysql", "../redis"]
  }
}
```

Similarly, in `frontend-app/terraform.tfvars`, you would specify:

```hcl
terragrunt = {
  dependencies {
    paths = ["../vpc", "../backend-app"]
  }
}
```

Once you've specified the dependencies in each `terraform.tfvars` file, when you run the `terragrunt apply-all` or 
`terragrunt destroy-all`, Terragrunt will ensure that the dependencies are applied or destroyed, respectively, in the
correct order. For the example at the start of this section, the order for the `apply-all` command would be:

1. Deploy the VPC
1. Deploy MySQL and Redis in parallel
1. Deploy the backend-app
1. Deploy the frontend-app

If any of the modules fail to deploy, then Terragrunt will not attempt to deploy the modules that depend on them. Once
you've fixed the error, it's usually safe to re-run the `apply-all` or `destroy-all` command again, since it'll be a 
no-op for the modules that already deployed successfully, and should only affect the ones that had an error the last 
time around.







## Terragrunt details

This section contains detailed documentation for the following aspects of Terragrunt:

1. [Helper functions](#helper-functions)
1. [CLI options](#cli-options)
1. [Migrating from Terragrunt v0.11.x and Terraform 0.8.x and older](#migrating-from-terragrunt-v011x-and-terraform-08x-and-older)
1. [Terragrunt config files](#terragrunt-config-files)
1. [Developing Terragrunt](#developing-terragrunt)
1. [License](#license)


### Helper functions

Terragrunt allows you to use the same inteprolation syntax as Terraform (`${...}`) to call *helper functions*. Note 
that helper functions *only* work within a `terragrunt = { ... }` block. Terraform does NOT process interpolations in 
`.tfvars` files.

Here are the supported helper functions:

* [find_in_parent_folders](#find_in_parent_folders)
* [path_relative_to_include](#path_relative_to_include)
* [get_env](#get_env)
* [get_tfvars_dir](#get_tfvars_dir)


#### find_in_parent_folders

`find_in_parent_folders()` searches up the directory tree from the current `.tfvars` file and returns the relative path
to to the first `terraform.tfvars` in a parent folder or exit with an error if no such file is found. This is 
primarily useful in an `include` block to automatically find the path to a parent `.tfvars` file:

```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```

  
#### path_relative_to_include

`path_relative_to_include()` returns the relative path between the current `.tfvars` file and the `path` specified in 
its `include` block. For example, consider the following folder structure:

```
├── terraform.tfvars
└── prod
    └── mysql
        └── terraform.tfvars
└── stage
    └── mysql
        └── terraform.tfvars
```

Imagine `prod/mysql/terraform.tfvars` and `stage/mysql/terraform.tfvars` include all settings from the root 
`terraform.tfvars` file:
 
```hcl
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
```
 
The root `terraform.tfvars` can use the `path_relative_to_include()` in its `remote_state` configuration to ensure 
each child stores its remote state at a different `key`: 

```hcl
terragrunt = {
  remote_state {
    backend = "s3" 
    config {
      bucket = "my-terraform-bucket"
      region = "us-east-1"
      key    = "${path_relative_to_include()}/terraform.tfstate"
    }
  }
}
```

The resulting `key` will be `prod/mysql/terraform.tfstate` for the prod `mysql` module and 
`stage/mysql/terraform.tfstate` for the stage `mysql` module. 


#### get_env

`get_env(NAME, DEFAULT)` returns the value of the environment variable named `NAME` or `DEFAULT` if that environment
variable is not set. Example: 

```hcl
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      bucket = "${get_env("BUCKET", "my-terraform-bucket")}"
    }
  }  
}
```  

Note that [Terraform will read environment 
variables](https://www.terraform.io/docs/configuration/environment-variables.html#tf_var_name) that start with the
prefix `TF_VAR_`, so one way to share the a variable named `foo` between Terraform and Terragrunt is to set its value
as the environment variable `TF_VAR_foo` and to read that value in using this `get_env` helper function.


#### get_tfvars_dir

`get_tfvars_dir()` returns the directory where the Terragrunt configuration file (by default, `terraform.tfvars`) lives. 
This is useful when you need to use relative paths with [remote Terraform 
configurations](#remote-terraform-configurations) and you want those paths relative to your Terragrunt configuration 
file and not relative to the temporary directory where Terragrunt downloads the code. 

For example, imagine you have the following file structure: 

```
/terraform-code
├── common.tfvars
├── frontend-app
│   └── terraform.tfvars
```

Inside of `/terraform-code/frontend-app/terraform.tfvars` you might try to write code that looks like this:

```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"
    
    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
      
      arguments = [
        "-var-file=../common.tfvars", # Note: This relative path will NOT work correctly!
        "-var-file=terraform.tfvars"
      ]
    }    
  }
}
```

Note how the `source` parameter is set, so Terragrunt will download the `frontend-app` code from the `modules` repo 
into a temporary folder and run `terraform` in that temporary folder. Note also that there is an `extra_arguments` 
block that is trying to allow the `frontend-app` to read some shared variables from a `common.tfvars` file. 
Unfortunately, the relative path (`../common.tfvars`) won't work, as it will be relative to the temporary folder! 
Moreover, you can't use an absolute path, or the code won't work on any of your teammates' computers.

To make the relative path work, you need to use the `get_tfvars_dir()` helper to combine the path with the folder where
the `.tfvars` file lives:

```hcl
terragrunt = {
  terraform {
    source = "git::git@github.com:foo/modules.git//frontend-app?ref=v0.0.3"
    
    extra_arguments "custom_vars" {
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
      
      # With the get_tfvars_dir helper, you can use relative paths! 
      arguments = [
        "-var-file=${get_tfvars_dir()}/../common.tfvars",
        "-var-file=terraform.tfvars"
      ]
    }    
  }
}
```

For the example above, this path will resolve to `/terraform-code/frontend-app/../common.tfvars`, which is exactly 
what you want.




### CLI Options

Terragrunt forwards all arguments and options to Terraform. The only exceptions are `--version` and arguments that 
start with the prefix `--terragrunt-`. The currently available options are:

* `--terragrunt-config`: A custom path to the `terraform.tfvars` file. May also be specified via the `TERRAGRUNT_CONFIG`
  environment variable. The default path is `terraform.tfvars` in the current directory (see 
  [Terragrunt config files](#terragrnt-config-files) for a slightly more nuanced explanation). This argument is not 
  used with the `apply-all`, `destroy-all`, and `output-all` commands.

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
  parameter. This argument is not used with the `apply-all`, `destroy-all`, and `output-all` commands. 

* `--terragrunt-source-update`: Delete the contents of the temporary folder before downloading Terraform source code
  into it.


### Terragrunt config files

The current version of Terragrunt expects configuration to be defined in a `terraform.tfvars` file. Previous
versions defined the config in a `.terragrunt` file. **The `.terragrunt` format is now deprecated**!

For backwards compatibility, Terragrunt will continue to support the `.terragrunt` file format for a short period of 
time. Check out the next section for how this works. Note that you will get a warning in your logs every time you run 
Terragrunt with a `.terragrunt` file, and we will eventually stop supporting this older format, so we recommend 
migrating to the `terraform.tfvars` format ASAP!


#### Config file search paths

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


#### Migrating from .terragrunt to terraform.tfvars

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


### Migrating from Terragrunt v0.11.x and Terraform 0.8.x and older

#### Background

Terragrunt was originally created to support two features that were not available in Terraform: defining remote state
configuration in a file (rather than via CLI commands) and locking. As of version 0.9.0, Terraform now supports both of 
these features natively, so we have had to make changes to Terragrunt: 

1. Terragrunt still supports remote state configuration so you can take advantage of Terragrunt's interpolation
   functions.
1. Terragrunt no longer supports locking.


#### Migration instructions

If you were using Terragrunt <= v0.11.x and Terraform <= 0.8.x, here is how to migrate:

1. In your Terraform code (the `.tf` files), you must now define a `backend`. For example, to use S3 as a remote state 
   backend, you will need to add the following to your Terraform code:
   
    ```hcl
    # main.tf
    terraform {
      # The configuration for this backend will be filled in by Terragrunt
      backend "s3" {}
    }
    ```
   
    Note that you can leave the configuration of the `backend` empty and allow Terragrunt to provide that configuration  
    instead. This allows you to keep your remote state configuration more DRY by taking advantage of Terragrunt's 
    interpolation functions:
    
    ```hcl
    # terraform.tfvars
    terragrunt = {
      remote_state {
        backend = "s3"
        config {
          bucket  = "my-terraform-state"
          key     = "${path_relative_to_include()}/terraform.tfstate"
          region  = "us-east-1"
          encrypt = true
        }
      }
    }
    ```    
   
1. Remove any `lock { ... }` blocks from your Terragrunt configurations, as these are no longer supported. 

    If you were storing remote state in S3 and relying on DynamoDB as a locking mechanism, Terraform now supports that 
    natively. To enable it, simply add the `lock_table` parameter to your S3 backend configuration. If you configure 
    your S3 backend using Terragrunt, then Terragrunt will automatically create the `lock_table` for you if that table
    doesn't already exist:

    ```hcl
    # terraform.tfvars
    terragrunt = {
      remote_state {
        backend = "s3" 
        config {
          bucket  = "my-terraform-state"
          key     = "${path_relative_to_include()}/terraform.tfstate"
          region  = "us-east-1"
          encrypt = true
       
          # Tell Terraform to do locking using DynamoDB. Terragrunt will automatically create this table for you if
          # it doesn't already exist. 
          lock_table = "my-lock-table"
        }
      }
    }
    ```

    If you would like Terraform to automatically retry locks like Terragrunt did (this is particularly useful when 
    running Terraform as part of an automated script, such as a CI build), you use an `extra_arguments` block:

    ```hcl
    # terraform.tfvars
    terragrunt = {
      remote_state {
        backend = "s3" 
        config {
          bucket  = "my-terraform-state"
          key     = "${path_relative_to_include()}/terraform.tfstate"
          region  = "us-east-1"
          encrypt = true
    
          # Tell Terraform to do locking using DynamoDB. Terragrunt will automatically create this table for you if
          # it doesn't already exist. 
          lock_table = "my-lock-table"
        }
      }
   
      # Force Terraform to keep trying to acquire a lock for up to 20 minutes if someone else already has the lock 
      extra_arguments "retry_lock" {
        commands = [
          "init",
          "apply",
          "refresh",
          "import",
          "plan",
          "taint",
          "untaint"
        ]
      
        arguments = [
          "-lock-timeout=20m"
        ]    
      }     
    }     
    ``` 

 

### Developing terragrunt

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


### License

This code is released under the MIT License. See LICENSE.txt.
