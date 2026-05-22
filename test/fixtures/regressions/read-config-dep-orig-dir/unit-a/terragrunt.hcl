include "root-common" {
  path           = "${get_terragrunt_dir()}/../common/root-common.hcl"
  expose         = false
  merge_strategy = "deep"
}
