# Overwrite because the existing file has a terragrunt signature.
generate "backend" {
  path      = "backend.tf"
  if_exists = "overwrite_terragrunt_or_skip"
  contents  = <<EOF
terraform {
  backend "local" {
    path = "foo.tfstate"
  }
}
EOF
}

terraform {
  source = "../../module"
}
