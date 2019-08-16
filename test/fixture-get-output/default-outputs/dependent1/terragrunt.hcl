dependency "source" {
  config_path = "../source"
  default_outputs = {
    the_answer = "0"
  }
}

inputs = {
  the_answer = dependency.source.outputs.the_answer
}
