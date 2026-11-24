locals {
  env = "live"
}

unit "db" {
  source = "../units/db"
  path   = "db"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "db" {
      config_path  = unit.db.path
      skip_outputs = true

      mock_outputs = {
        consumer = unit.path
      }
    }

    inputs = {
      auto_unit   = unit.path
      auto_module = path.download
      auto_stack  = stack.path
      auto_label  = "${local.env}:${unit.path}"
      db_consumer = dependency.db.outputs.consumer
    }
  }
}
