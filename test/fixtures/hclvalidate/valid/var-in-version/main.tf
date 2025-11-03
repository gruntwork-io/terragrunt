locals {
  variable_version = "0.0.0"
}

module "module" {
  source  = "github.com/foo/bar"
  version = local.variable_version
}
