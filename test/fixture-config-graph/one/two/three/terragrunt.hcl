locals {
  full-name = "${local.name}-${local.region}"
  name = "test"
  region = "us-east-1"
  parent = "${local.parent-dir}/terragrunt.hcl"
  parent-dir = "../../.."
}
globals {
  region = local.region
  source-postfix = "${local.parent}-${include.relative}"
}
include {
  path = "${local.parent}"
}
input = {
  region = global.region
}
