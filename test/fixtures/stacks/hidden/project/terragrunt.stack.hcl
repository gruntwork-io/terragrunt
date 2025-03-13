stack "stack-config" {
  source = "${get_repo_root()}/config"
  path   = "stack-config"
  hidden = true
}

unit "unit-config" {
  source = "${get_repo_root()}/config"
  path   = "unit-config"
  hidden = true
}

stack "dev" {
  source = "${get_repo_root()}/stacks/dev"
  path   = "dev"
}

