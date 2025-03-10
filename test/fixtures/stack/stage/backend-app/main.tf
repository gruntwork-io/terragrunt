terraform {
  backend "s3" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "3.2.3"
    }
  }
}

# Create an arbitrary local resource
resource "null_resource" "text" {
  provisioner "local-exec" {
    command = "echo '[I am a backend-app template. Data from my dependencies: vpc = ${data.terraform_remote_state.vpc.outputs.text}, bastion-host = ${data.terraform_remote_state.bastion_host.outputs.text}, mysql = ${data.terraform_remote_state.mysql.outputs.text}, search-app = ${data.terraform_remote_state.search_app.outputs.text}]'"
  }
}

output "text" {
  value = "[I am a backend-app template. Data from my dependencies: vpc = ${data.terraform_remote_state.vpc.outputs.text}, bastion-host = ${data.terraform_remote_state.bastion_host.outputs.text}, mysql = ${data.terraform_remote_state.mysql.outputs.text}, search-app = ${data.terraform_remote_state.search_app.outputs.text}]"
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
