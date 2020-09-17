terraform {
  backend "s3" {}
}

# Create an arbitrary local resource
data "template_file" "text" {
  template = "[I am a search-app template. Data from my dependencies: vpc = ${data.terraform_remote_state.vpc.outputs.text}, redis = ${data.terraform_remote_state.redis.outputs.text}, example_module = ${module.example_module.text}]"
}

module "example_module" {
  source = "./example-module"
}

output "text" {
  value = data.template_file.text.rendered
}

variable "terraform_remote_state_s3_bucket" {
  description = "The name of the S3 bucket where Terraform remote state is stored"
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
