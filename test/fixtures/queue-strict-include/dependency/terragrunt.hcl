terraform {
  source = "."
}

dependency "transitive_dep" {
  config_path = "../transitive-dependency"

  mock_outputs = {}
}

