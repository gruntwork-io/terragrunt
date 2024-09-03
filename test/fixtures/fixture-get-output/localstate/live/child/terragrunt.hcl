include {
  path = find_in_parent_folders()
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
