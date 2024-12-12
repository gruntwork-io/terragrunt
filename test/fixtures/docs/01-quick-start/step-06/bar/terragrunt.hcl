terraform {
  source = "../shared"
}

dependency "foo" {
  config_path = "../foo"
}

inputs = {
  content = "Foo content: ${dependency.foo.outputs.content}"
}
