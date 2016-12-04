# Create an arbitrary local resource
data "template_file" "text" {
  template = "[I am a backend-app template. Data from my dependencies: vpc = ${data.terraform_remote_state.vpc.text}, bastion-host = ${data.terraform_remote_state.bastion_host.text}, mysql = ${data.terraform_remote_state.mysql.text}, search-app = ${data.terraform_remote_state.search_app.text}]"
}

output "text" {
  value = "${data.template_file.text.rendered}"
}

variable "terraform_remote_state_s3_bucket" {
  description = "The name of the S3 bucket where Terraform remote state is stored"
}

data "terraform_remote_state" "vpc" {
  backend = "s3"
  config {
    region = "us-west-2"
    bucket = "${var.terraform_remote_state_s3_bucket}"
    key = "stage/vpc/terraform.tfstate"
  }
}

data "terraform_remote_state" "mysql" {
  backend = "s3"
  config {
    region = "us-west-2"
    bucket = "${var.terraform_remote_state_s3_bucket}"
    key = "stage/mysql/terraform.tfstate"
  }
}

data "terraform_remote_state" "search_app" {
  backend = "s3"
  config {
    region = "us-west-2"
    bucket = "${var.terraform_remote_state_s3_bucket}"
    key = "stage/search-app/terraform.tfstate"
  }
}

data "terraform_remote_state" "bastion_host" {
  backend = "s3"
  config {
    region = "us-west-2"
    bucket = "${var.terraform_remote_state_s3_bucket}"
    key = "mgmt/bastion-host/terraform.tfstate"
  }
}
