locals {
  app = read_terragrunt_config("../app/terragrunt.hcl")
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    app_stack = local.app.inputs.stack
    app_db    = local.app.inputs.db
  }

  autoinclude {
    inputs = {
      auto_app_stack = local.app.inputs.stack
    }
  }
}
