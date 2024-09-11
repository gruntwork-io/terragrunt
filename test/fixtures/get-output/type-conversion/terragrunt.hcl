terraform {
  source = "${get_terragrunt_dir()}/../../inputs"
}

dependency "inputs" {
  config_path = "../../inputs"
}

inputs = dependency.inputs.outputs
