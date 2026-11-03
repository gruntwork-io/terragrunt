dependency "db" {
  config_path = "../db/terragrunt.hcl"

  mock_outputs = {
    a = "mock"
  }
}

inputs = {
  a = dependency.db.outputs.a
}
