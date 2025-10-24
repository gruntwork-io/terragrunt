terraform {
  backend "s3" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

# Create an arbitrary local resource
resource "null_resource" "text" {
  provisioner "local-exec" {
    command = "echo '[I am a stage vpc template. Data from my dependencies: vpc = ${data.terraform_remote_state.mgmt_vpc.outputs.text}]'"
  }
}

output "text" {
  value = "[I am a stage vpc template. Data from my dependencies: vpc = ${data.terraform_remote_state.mgmt_vpc.outputs.text}]"
}

variable "terraform_remote_state_s3_bucket" {
  description = "The name of the S3 bucket where Terraform remote state is stored"
  type        = string
}

data "terraform_remote_state" "mgmt_vpc" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "mgmt/vpc/terraform.tfstate"
  }
}
