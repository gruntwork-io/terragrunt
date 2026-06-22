locals {
  env = values.env
}

unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "${local.env}-vpc"
}
