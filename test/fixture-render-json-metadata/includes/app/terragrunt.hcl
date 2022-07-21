include "common" {
  path   = "../common/common.hcl"
}

include "inputs" {
  path   = "./inputs.hcl"
}

include "locals" {
  path   = "./locals.hcl"
}

include "generate" {
  path   = "./generate.hcl"
}

locals {
  abc = "xyz"

}