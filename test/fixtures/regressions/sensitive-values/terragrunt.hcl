locals {
  environment = "dev"

  password = {
    dev = sensitive(yamldecode(file("dev.enc.yaml")).password)
  }[local.environment]
}

inputs = {
  password = local.password
}

terraform {
  source = "."
}
