exclude {
  if = true
  actions = ["all"]
  no_run = true
}

terraform {
  source = "../module"
}

include "foo" {
  path = "foo.hcl"
}

inputs = {
  input = "first"
}