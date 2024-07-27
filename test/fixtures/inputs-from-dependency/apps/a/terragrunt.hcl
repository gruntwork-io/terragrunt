terraform {
  source = "${get_terragrunt_dir()}/../../modules/a"
}

dependencies {
  paths = ["../b"]
}

dependency "b" {
  config_path = "../b"
}

inputs = {
  foo = dependency.b.outputs.foo
  bar = dependency.b.inputs.bar
  baz = dependency.b.inputs.baz
}
