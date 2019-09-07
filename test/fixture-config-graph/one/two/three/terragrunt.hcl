locals {
  full-name = "${local.name}-${local.region}"
  name = "test"
  region = "us-east-1"
  parent = "${local.parent}/terragrunt.hcl"
  parent-dir = "../../.."
}
globals {
  region = local.region
}
include {
  path = "${local.parent}"
}
input = {
  region = global.region
}
