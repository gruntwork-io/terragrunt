terraform {
  source = "../shared"
}

dependency "foo" {
  config_path = "../foo"

  mock_outputs = {
    content = "Mocked content from foo"
  }

  mock_outputs_allowed_terraform_commands = ["plan"]
}

inputs = {
  content = "Foo content: ${dependency.foo.outputs.content}"
}
