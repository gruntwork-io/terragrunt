terraform {
  source = "../../modules//app"
}

dependency "dep" {
  config_path = "../dependency"
}

inputs = {
  foo = dependency.dep.outputs.foo
}
