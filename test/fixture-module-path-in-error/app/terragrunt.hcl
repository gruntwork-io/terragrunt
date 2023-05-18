dependency "d1" {
  config_path = "../d1"

  mock_outputs = {
    d1 = "d1-value"
  }
}

dependency "d2" {
  config_path = "../d2"

  mock_outputs = {
    d2 = "d2-value"
  }
}
