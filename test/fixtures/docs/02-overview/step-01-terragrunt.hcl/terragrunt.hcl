# Configure the remote backend
remote_state {
  backend = "s3"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    bucket = "__FILL_IN_BUCKET_NAME__"

    key            = "tofu.tfstate"
    region         = "__FILL_IN_REGION__"
    encrypt        = true
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
  }
}

# Configure the AWS provider
generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "aws" {
  region = "__FILL_IN_REGION__"
}
EOF
}

# Configure the module
#
# The URL used here is a shorthand for
# "tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=5.16.0".
#
# You can find the module at:
# https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest
#
# Note the extra `/` after the `tfr` protocol is required for the shorthand
# notation.
terraform {
  source = "tfr:///terraform-aws-modules/vpc/aws?version=5.16.0"
}

# Configure the inputs for the module
inputs = {
  name = "step-one-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway = false
  enable_vpn_gateway = false

  tags = {
    IaC = "true"
    Environment = "dev"
  }
}

