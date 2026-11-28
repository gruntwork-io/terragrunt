dependency "db" {
  config_path  = "../db"
  skip_outputs = true

  mock_outputs = {
    input = commands.accept_input
  }
}

inputs = {
  db_input = dependency.db.outputs.input
}
