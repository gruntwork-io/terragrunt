dependency "y" {
  config_path = "./y"

  mock_outputs = {
    value = "mock value"
  }
}

terraform {
  source = "."
}
