include {
  path = find_in_parent_folders("root.hcl")
}

dependency "x" {
  config_path = "../parent"

  mock_outputs_allowed_terraform_commands = ["output", "validate", "init", "destroy", "plan", "apply", "terragrunt-info"]
  mock_outputs = {
    test_output1 = "fake-data"
    test_output2 = "fake-data2"
  }
}

inputs = {
  test_input1 = dependency.x.outputs.test_output1
  test_input2 = dependency.x.outputs.test_output2
}

terraform {
  source = "../..//modules/child"
}
