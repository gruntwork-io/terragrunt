dependency "source" {
  config_path = "../source"
  mock_outputs = {
    the_answer = "0"
  }
}

inputs = {
  the_answer = dependency.source.outputs.the_answer
}
