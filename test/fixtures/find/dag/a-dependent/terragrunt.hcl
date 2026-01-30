dependency "dep" {
  config_path = "../b-dependency"

  mock_outputs = {
    value = "mock value"
  }
}

terraform {
  source = "."
}
