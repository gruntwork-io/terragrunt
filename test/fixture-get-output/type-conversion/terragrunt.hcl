terraform {
  source = "${get_terragrunt_dir()}/../../fixture-inputs"
}

terragrunt_output "inputs" {
  config_path = "../../fixture-inputs"
}

inputs = terragrunt_output.inputs
