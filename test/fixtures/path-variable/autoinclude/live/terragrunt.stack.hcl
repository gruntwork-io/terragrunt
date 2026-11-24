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
        consumer = path.unit
      }
    }

    inputs = {
      auto_unit   = path.unit
      auto_module = path.download
      auto_stack  = path.parentstack
      auto_label  = "${local.env}:${path.unit}"
      db_consumer = dependency.db.outputs.consumer
    }
  }
}
