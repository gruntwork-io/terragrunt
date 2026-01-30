dependency "dep" {
  config_path = "../dependency-unit"

  mock_outputs = {}
}

terraform {
  source = "."
}

