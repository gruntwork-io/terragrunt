generate "backend" {
  path      = "main.tf"
  if_exists = "skip"
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
