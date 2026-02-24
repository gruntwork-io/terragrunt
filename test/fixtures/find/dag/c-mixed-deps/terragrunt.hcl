# Uses both dependency and dependencies blocks
dependency "single_dep" {
  config_path = "../a-dependent"

  mock_outputs = {
    value = "mock value"
  }
}

dependencies {
  paths = ["../d-dependencies-only"]
}

terraform {
  source = "."
}
