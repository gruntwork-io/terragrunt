dependency "unit_without_enabled" {
  config_path  = "../unit-without-enabled"
  mock_outputs = {
    "output1" = "mocked_output1"
  }
}

dependency "unit_disabled" {
  config_path = "../unit-disabled"
  enabled     = false

  mock_outputs = {
    "output2" = "mocked_output2"
  }
}

dependency "unit_enabled" {
  config_path = "../unit-enabled"
  enabled     = true

  mock_outputs = {
    "output3" = "mocked_output3"
  }
}

dependency "unit_missing" {
  config_path = "../unit-missing"
  enabled     = false
}
