terraform {
  backend "s3" {}
}

# Create an arbitrary local resource
data "template_file" "text" {
  template = "[I am a stage vpc template. Data from my dependencies: vpc = ${data.terraform_remote_state.mgmt_vpc.outputs.text}]"
}

output "text" {
  value = data.template_file.text.rendered
}

variable "terraform_remote_state_s3_bucket" {
  description = "The name of the S3 bucket where Terraform remote state is stored"
}

data "terraform_remote_state" "mgmt_vpc" {
  backend = "s3"
  config = {
    region = "us-west-2"
    bucket = var.terraform_remote_state_s3_bucket
    key    = "mgmt/vpc/terraform.tfstate"
  }
}
