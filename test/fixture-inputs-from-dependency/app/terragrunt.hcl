dependencies {
  paths = ["../dependency"]
}

dependency "dep" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.dep.inputs.foo
  bar = dependency.dep.inputs.bar
  baz = dependency.dep.inputs.baz

  dep-output-test = dependency.dep.outputs.test
  dep-cluster-id  = dependency.dep.outputs.cluster-id
}
