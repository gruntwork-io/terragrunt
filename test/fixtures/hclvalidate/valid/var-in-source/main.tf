locals {
  variable_source = "github.com/foo/bar"
}

module "module" {
  source  = local.variable_source
  version = "0.0.0"
}
