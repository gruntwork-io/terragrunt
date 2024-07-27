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
  program = [
    "jq", "-n",
    "--arg", "vpc", data.terraform_remote_state.vpc.outputs.text,
    "--arg", "bastion_host", data.terraform_remote_state.bastion_host.outputs.text,
    "--arg", "mysql", data.terraform_remote_state.mysql.outputs.text,
    "--arg", "search_app", data.terraform_remote_state.search_app.outputs.text,
    "{\"text\": \"[I am a backend-app template. Data from my dependencies: vpc = \\($vpc), bastion-host = \\($bastion_host), mysql = \\($mysql), search-app = \\($search_app)]\"}"
  ]
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

data "terraform_remote_state" "mysql" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "stage/mysql/terraform.tfstate"
  }
}

data "terraform_remote_state" "search_app" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "stage/search-app/terraform.tfstate"
  }
}

data "terraform_remote_state" "bastion_host" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "mgmt/bastion-host/terraform.tfstate"
  }
}
