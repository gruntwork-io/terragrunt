terraform {
  source = "${get_terragrunt_dir()}/../../fixture-inputs"
}

inputs = get_output("../../fixture-inputs")
