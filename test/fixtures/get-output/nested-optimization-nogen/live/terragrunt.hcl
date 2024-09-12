# Retrieve a dependency. In the test, we will destroy this state and verify we can still get the output.
dependency "dep" {
  config_path = "../dep"
}

inputs = {
  input = dependency.dep.outputs.output
}
