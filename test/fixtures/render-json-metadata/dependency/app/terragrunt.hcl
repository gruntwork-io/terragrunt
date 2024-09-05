
dependency "dep" {
  config_path = "../dependency"
      
    mock_outputs = {
      test = "value"
    }
}

dependency "dep2" {
  config_path = "../dependency2"

  mock_outputs = {
    test2 = "value2"
  }
}
