dependency "m1" {
  config_path  = "../m1"
  mock_outputs = {
    "output1" = "mocked_output1"
  }
}

dependency "m2" {
  config_path = "../m2"
  enabled     = false

  mock_outputs = {
    "output2" = "mocked_output2"
  }
}

dependency "m3" {
  config_path = "../m3"
  enabled     = true

  mock_outputs = {
    "output3" = "mocked_output3"
  }
}
