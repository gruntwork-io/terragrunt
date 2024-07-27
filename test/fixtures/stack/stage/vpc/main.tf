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
  program = ["jq", "-n", "--arg", "vpc", data.terraform_remote_state.mgmt_vpc.outputs.text, "{\"text\": \"Example text from a module. Data from dependencies: vpc = \\($vpc)\"}"]
}

output "text" {
  value = data.external.text.result.text
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
