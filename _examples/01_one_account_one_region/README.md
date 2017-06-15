# WIP

This example demonstrates several basic Terragrunt best practices by
creating a simple infrastructure with two environments (production and staging),
which is hosted in a single AWS account in a single AWS region.

**Demonstrated concepts and keywords:**

* Recommended Terragrunt directory structure
* Basic Terragrunt configuration in `terraform.tfvars` files
* Keeping Terragrunt configurations DRY by using the `include` keyword and 
  the `find_in_parent_folders()` Terragrunt built-in function
* Using remotely sourced and independently versioned Terraform modules
* Independent remote state for each individual infrastructure component
* Defining dependencies between infrastructure components using the Terragrunt 
  `dependencies` keyword
* Sharing remote state between infrastructure components using `terraform_remote_state`

## Running the example
To create the staging environment:
```
$> cd live/staging
$> terragrunt apply-all
```


To create the production environment:
```
$> cd live/production
$> terragrunt apply-all
```

## Directory structure
TODO: move the discussion of directory hierarchy and definitions out of the root
`terraform.tfvars` file and put it here instead?

## Each infrastructure component is defined in a Terraform module
In this example, our infrastructure contains two components: a VPC and a frontend_app.
Each of these components is defined in its own Terraform module:
[conorgil/terragrunt_sample_vpc](https://github.com/conorgil/terragrunt_sample_vpc)
and
[conorgil/terragrunt_sample_frontend_app](https://github.com/conorgil/terragrunt_sample_frontend_app)
respectively.

There are a few important things to note about the Terraform modules:
1. The code in each of these repos is regular plain old Terraform code. These
   Terraform modules can be used without modification in any project that does
   not use Terragrunt and instead uses Terarform directly.
1. Each module defines a variable for any value that might change between invocations
   of the module. For example, values that might change between production and staging
   environments.
1. There are no `terraform.tfvars` files in the Terraform module repos. The Terraform
   modules are generic infrastructure templates and do not define values for any
   variables in `*.tfvar` files. The values for variables are defined in a separate
   repo where Terragrunt conifguration also lives. In this example, you can see that
   the values for variables are defined in this repo in the `terraform.tfvars` file
   in each component directory.

Each component of our infrastructure will be defined in its own Terraform module
in the same way. This provides several benefits:
1. Versioning
   1. Hosting each component in its own repo means that each component can be indepdently
      versioned, which allows each environment in the infrastructure to use a different
      version of the Terraform module. This allows for straightforward development and
      testing of Terraform module changes in QA and staging environments before applying
      those changes to production.
     
      In this example, look at the `source` stanza of the `terraform` block in any of the
      component configuration files. Notice that each specifies a specific version of the
      Terraform module source by using the `?ref=<some version>` syntax.
2. Independent remote state
   1. Typically, a Terraform project will have a single top level `main.tf` file
      for each environment, which calls a Terraform module for each component of
      the infrastructure. As a result of having a single top level `main.tf` file,
      the entire environment has a single remote state file.
   
      Hosting each Terraform module in its own repo makes it possible to setup the directory
      structure such that each component within an environment has its own remote state file.
      In this example, each component is defined in its own directory. This means that when
      Terraform is run in each directory, a remote state file will be created for only the
      infrastructure defined within that directory; a single component within the environment.
      
      Having a remote state file for each component makes it easier to make changes to a single
      component; commands like `plan` and `apply` will be quicker because they only have to refresh
      the state of the single component, not the entire environment. Also, each state file is smaller
      in scope, which means that we have a smaller blast radius if a bug is introduced into one of
      our components. It is more likely to break a single component instead of the entire environment.
      
## Dependencies between infrastructure components
It might be best to come up with some contrived example since this is the first one and *NOT* introduce
the `dependencies` keyword yet. Save it for a future example so that we can explain it appropriately?

Some things to discuss with the `dependencies` keyword:
* makes `apply-all` and `destroy-all` work as expected
* issues with `plan-all`

If we do leave this in the first example, this section would also explain that the dependency
between the two modules is because the
[conorgil/terragrunt_sample_frontend_app](https://github.com/conorgil/terragrunt_sample_frontend_app)
Terraofrm module needs to pull from the 
[conorgil/terragrunt_sample_vpc](https://github.com/conorgil/terragrunt_sample_vpc)
module which subnet to use for the EC2 instance. Highlight that the 
[conorgil/terragrunt_sample_frontend_app](https://github.com/conorgil/terragrunt_sample_frontend_app)
module accomplishes this by using a `terraform_remote_state` data source and exposes the configuration
for that resource as variables.
