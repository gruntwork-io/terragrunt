dependencies {
  paths = ["../dependency"]
}

dependency "test" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.test.inputs.foo
}
