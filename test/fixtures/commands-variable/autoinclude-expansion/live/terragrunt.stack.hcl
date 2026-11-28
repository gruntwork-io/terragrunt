unit "db" {
  source = "../../autoinclude/units/app"
  path   = "db"
}

unit "app" {
  source = "../../autoinclude/units/app"
  path   = "app"

  autoinclude {
    dependency "db" {
      expansion {
        for_each = toset(commands.accept_parallelism)
      }

      config_path  = unit.db.path
      skip_outputs = true

      mock_outputs = {
        command = each.key
      }
    }

    inputs = {
      db_commands = [for name, db in dependency.db : db.outputs.command]
    }
  }
}
