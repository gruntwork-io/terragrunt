include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "."
}

# The outputs are deliberately unreferenced: mock_outputs that can't be keyed by unit name has to
# fail while the stack outputs are collected, rather than passing here and only surfacing wherever
# the outputs happen to be read.
dependency "networking" {
  config_path = "../networking"

  mock_outputs = "not-keyed-by-unit-name"
}
