locals {
  parent_var = run_cmd("echo", "dependency_hcl")
}

include "common" {
  path = "../common.hcl"
}
