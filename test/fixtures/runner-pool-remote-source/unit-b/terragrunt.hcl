terraform {
  source = "git::__MIRROR_URL__//test/fixtures/fail-fast/unit-a?ref=v0.84.1"
}

dependency "unit-a" {
  config_path = "../unit-a"
  mock_outputs = {
    data = "test-data"
  }
}

inputs = {
  data = dependency.unit-a.outputs.data
}
