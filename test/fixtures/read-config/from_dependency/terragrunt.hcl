dependency "dep" {
  config_path = "./dep/"
}

inputs = {
  bar = dependency.dep.outputs.foo
}
