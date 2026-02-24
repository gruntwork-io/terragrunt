terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dependency"

  mock_outputs = {}
}
