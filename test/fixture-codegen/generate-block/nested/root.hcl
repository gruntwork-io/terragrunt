generate "backend" {
  path      = "backend.tf"
  if_exists = "overwrite"
  contents  = <<EOF
terraform {
  backend "local" {
    path = "foo.tfstate"
  }
}
EOF
}

terraform {
  source = "${get_parent_terragrunt_dir()}/../../module"
}
