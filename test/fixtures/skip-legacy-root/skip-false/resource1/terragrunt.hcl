include {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../base-module"
}

inputs = {
  person = "Ernie"
}

