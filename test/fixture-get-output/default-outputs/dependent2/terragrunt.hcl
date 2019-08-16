dependency "source" {
  config_path = "../source"
  default_outputs = {
    the_answer = "0"
  }
  default_outputs_allowed_terraform_commands = ["validate"]
}

inputs = {
  the_answer = dependency.source.outputs.the_answer
}
