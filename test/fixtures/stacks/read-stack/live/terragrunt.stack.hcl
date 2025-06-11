locals {
  project = "test-project"
  version = "6.6.6"
  env     = "test"
}

unit "test_app" {
  source = "../units/app"
  path   = "app"
  values = {
    app     = "test-app"
    project = local.project
    version = local.version
    env     = local.env
    data    = "test"
  }
}

stack "dev" {
  source = "${get_repo_root()}/stacks/dev"
  path   = "dev"
  values = {
    project = "dev-project"
    env     = "dev"
  }
}

stack "prod" {
  source = "${get_repo_root()}/stacks/prod"
  path   = "prod"
  values = {
    project = "prod-project"
    env     = "prod"
  }
}

