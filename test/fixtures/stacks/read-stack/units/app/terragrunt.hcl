locals {
  read_stack = read_terragrunt_config("${get_repo_root()}/live/terragrunt.stack.hcl")
  read_values = read_terragrunt_config("terragrunt.values.hcl")
}

inputs = {
  stack_local_project = local.read_stack.local.project
  unit_source         = local.read_stack.unit.test_app.source
  unit_name           = local.read_stack.unit.test_app.name
  unit_value_version  = local.read_stack.unit.test_app.values.version
  stack_source        = local.read_stack.stack.dev.source
  stack_value_env     = local.read_stack.stack.dev.values.env
  project             = "${local.read_values.project}"
  env                 = local.read_values.env
  data                = local.read_values.data
}