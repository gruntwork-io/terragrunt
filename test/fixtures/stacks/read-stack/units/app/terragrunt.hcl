locals {
  stack  = read_terragrunt_config("${get_repo_root()}/live/terragrunt.stack.hcl")
  values = read_terragrunt_config("terragrunt.values.hcl")
}

inputs = {
  project = "local: ${local.values.project} stack: ${local.stack.local.project}"
  env     = local.values.env
  data    = local.values.data
}