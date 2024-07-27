terraform {
  backend "s3" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

# Create an arbitrary local resource
data "external" "text" {
  program = ["jq", "-n", "--arg", "vpc", data.terraform_remote_state.vpc.outputs.text, "--arg", "redis", data.terraform_remote_state.redis.outputs.text, "--arg", "example_module", module.example_module.text, "{\"text\": \"Example text from a module. Data from dependencies: vpc = \\($vpc), redis = \\($redis), example_module = \\($example_module)\"}"]
}

module "example_module" {
  source = "./example-module"
}

output "text" {
  value = data.external.text.result.text
}

variable "terraform_remote_state_s3_bucket" {
  description = "The name of the S3 bucket where Terraform remote state is stored"
  type        = string
}

data "terraform_remote_state" "vpc" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "stage/vpc/terraform.tfstate"
  }
}

data "terraform_remote_state" "redis" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "stage/redis/terraform.tfstate"
  }
}
