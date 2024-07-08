dependency "a" {
  config_path = "${path_relative_from_include()}/${path_relative_to_include()}/../a"

  # skip_outputs = false
  # mock_outputs = {
  #   z = "mocked-a"
  # }
}
