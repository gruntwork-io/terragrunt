include {
  path = find_in_parent_folders("root.hcl")
}

# Retrieve a dependency. In the test, we will destroy this state and verify we can still get the output.
dependency "deepdep" {
  config_path = "../deepdep"
}

inputs = {
  input = dependency.deepdep.outputs.output
}
