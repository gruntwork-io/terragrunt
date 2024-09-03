include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-dirs?ref=v0.35.1"
}

dependency "common" {
  config_path = "../../common"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs = {
    vpc_id = "fake-vpc-id"
  }

  skip_outputs = "true"
}


generate "provider" {
  path      = "providers.tf"
  if_exists = "overwrite"
  contents = <<EOF
provider "aws" {
  region              = "us-east-1"
}
EOF
}


generate "outputs_1" {
  path      = "outputs_1.tf"
  if_exists = "overwrite"
  contents = <<EOF
output "outputs_1" {
    value = "outputs_1"
}
EOF
}

generate "outputs_2" {
  path      = "outputs_2.tf"
  if_exists = "overwrite"
  contents  = <<EOF
output "outputs_2" {
    value = "outputs_2"
}
EOF
}
