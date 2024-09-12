dependency "vpc" {
  config_path = ""
  mock_outputs = {
    attribute     = "will be replaced"
    new_attribute = "new val"
    list_attr     = ["mock"]
    map_attr = {
      bar = "baz"
    }
  }
}
