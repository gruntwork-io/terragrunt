dependencies {
  paths = ["../dependency"]
}

dependency "test" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.test.inputs.foo
  bar = dependency.test.inputs.bar
  baz = dependency.test.inputs.baz
}
