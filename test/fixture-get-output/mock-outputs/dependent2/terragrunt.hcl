dependency "source" {
  config_path = "../source"
  mock_outputs = {
    the_answer = "0"
  }
  mock_outputs_allowed_terraform_commands = ["validate"]
}

inputs = {
  the_answer = dependency.source.outputs.the_answer
}
