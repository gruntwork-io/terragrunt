dependency "db" {
  config_path = "../db"

  mock_outputs = {
    id = "mock"
  }
}

inputs = {
  stack = try(path.stack, "no-stack")
  db    = dependency.db.outputs.id
}
