skip = true

terraform {
  source = "../module"
}

include "foo" {
  path = "foo.hcl"
}

inputs = {
  input = "first"
}