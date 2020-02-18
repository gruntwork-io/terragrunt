include {
  path = "${get_terragrunt_dir()}/../root.hcl"
}

generate "backend" {
  path      = "backend.tf"
  if_exists = "overwrite"
  contents  = <<EOF
terraform {
  backend "local" {
    path = "bar.tfstate"
  }
}
EOF
}

generate "random_file" {
  path = "random_file.txt"
  if_exists = "overwrite"
  contents = "Hello world"
}
