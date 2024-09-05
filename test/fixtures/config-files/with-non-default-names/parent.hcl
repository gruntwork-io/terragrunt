locals {
  parent_var = run_cmd("echo", "parent_hcl_file")
}

dependency "dependency" {
  config_path = "../dependency/another-name.hcl"

  mock_outputs = {
    mock_key = "mock_value"
  }

}
