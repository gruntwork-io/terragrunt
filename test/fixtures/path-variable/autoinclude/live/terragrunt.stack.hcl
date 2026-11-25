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
        consumer = path.stack
      }
    }

    inputs = {
      auto_stack  = path.stack
      auto_unit   = try(path.unit, "no-unit")
      auto_label  = "${local.env}:${path.stack}"
      auto_loop   = [for path in [path.stack] : "${path}:${dependency.db.outputs.consumer}"]
      db_consumer = dependency.db.outputs.consumer
    }
  }
}
