locals {
  layer_config = read_terragrunt_config("../layer/layer.hcl")
}

inputs = {
  name_from_layer = local.layer_config.inputs.dep_name
}