
stack "dev" {
  source = "${get_repo_root()}/stacks/dev"
  path = "dev"
  values = {
    project = "dev-project"
    env = "dev"
  }
}

stack "prod" {
  source = "${get_repo_root()}/stacks/prod"
  path = "prod"
  values = {
      project = "prod-project"
      env = "prod"
  }
}

