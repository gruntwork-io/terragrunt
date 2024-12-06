include {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../modules/child"
}

dependency "x" {
  config_path = "../parent"
}

inputs = {
  x = dependency.x.outputs.x
}
