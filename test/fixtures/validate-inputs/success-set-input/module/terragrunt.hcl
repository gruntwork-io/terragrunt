dependency "some_dependency" {
  config_path = "../dep"
}

inputs = {
  my_test_variable = toset([
    dependency.some_dependency.outputs.id,
    dependency.some_dependency.outputs.name,
  ])
}
