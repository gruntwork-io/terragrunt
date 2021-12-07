include {
  path = find_in_parent_folders()
}

dependency "x" {
  config_path = "../parent"

  mock_outputs_merge_with_state = "false"
  mock_outputs_allowed_terraform_commands = ["plan", "apply", "output"]
  mock_outputs = {
    test_output1 = "fake-output1"
    test_output_map_map_string = {
      map_root1 = {
        map_root1_sub1 = "fake-map_root1_sub1"
      }
      not_in_state = {
        abc = "fake-abc"
      }
    }
    test_output_list_string = ["fake-list-data"]
  }
}

inputs = {
  test_input1 = dependency.x.outputs.test_output1

  test_input_map_map_string = dependency.x.outputs.test_output_map_map_string

  test_input_list_string = dependency.x.outputs.test_output_list_string
}

terraform {
  source = "../..//modules/child"
}
