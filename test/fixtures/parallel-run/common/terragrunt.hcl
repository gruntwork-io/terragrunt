terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-dirs?ref=v0.35.1"
}

generate "providers" {
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
  contents = <<EOF
output "outputs_2" {
    value = "outputs_2"
}
EOF
}

generate "outputs_3" {
  path      = "outputs_3.tf"
  if_exists = "overwrite"
  contents = <<EOF
output "outputs_3" {
    value = "outputs_3"
}
EOF
}

generate "outputs_4" {
  path      = "outputs_4.tf"
  if_exists = "overwrite"
  contents = <<EOF
output "outputs_4" {
    value = "outputs_4"
}
EOF
}

generate "outputs_5" {
  path      = "outputs_5.tf"
  if_exists = "overwrite"
  contents = <<EOF
output "outputs_5" {
    value = "outputs_5"
}
EOF
}
