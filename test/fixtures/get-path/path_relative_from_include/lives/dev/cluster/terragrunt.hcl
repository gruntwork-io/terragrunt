terraform {
  source = "../../../modules//cluster"
}

include {
  path = find_in_parent_folders()
}

dependency "base" {
  config_path = "../base"
}

inputs = {
  some_input = dependency.base.outputs.some_output
}
