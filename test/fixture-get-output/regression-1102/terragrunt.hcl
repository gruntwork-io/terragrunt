terraform {
  source = "${get_terragrunt_dir()}/../../fixture-inputs"
}

dependency "inputs" {
  config_path = "../../fixture-inputs"
}

inputs = dependency.inputs.outputs
