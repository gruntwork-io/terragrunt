stack "stack-config" {
  source                  = "${get_repo_root()}/config"
  path                    = "stack-config"
  no_dot_terragrunt_stack = true
}

unit "unit-config" {
  source                  = "${get_repo_root()}/config"
  path                    = "unit-config"
  no_dot_terragrunt_stack = true
}

stack "dev" {
  source = "${get_repo_root()}/stacks/dev"
  path   = "dev"
}

