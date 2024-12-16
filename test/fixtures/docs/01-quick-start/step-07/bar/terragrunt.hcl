terraform {
  source = "../shared"
}

dependency "foo" {
  config_path = "../foo"

  mock_outputs = {
    content = "Mocked content from foo"
  }
}

inputs = {
  content = "Foo content: ${dependency.foo.outputs.content}"
}
