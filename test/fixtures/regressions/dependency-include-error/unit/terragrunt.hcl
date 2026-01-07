# Unit that includes both root.hcl and layer.hcl
# This reproduces issue #5169 where include directive parsing
# shows false positive "Unknown variable" errors

include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "layer" {
  path = find_in_parent_folders("layer.hcl")
}
