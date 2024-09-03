dependency "vpc" {
  # This will get overridden by child terragrunt.hcl configs
  config_path = ""

  mock_outputs = {
    attribute     = "hello"
    old_attribute = "old val"
    list_attr     = ["hello"]
    map_attr = {
      foo = "bar"
    }
  }
  mock_outputs_allowed_terraform_commands = ["apply", "plan", "destroy", "output"]
}

inputs = {
  attribute     = "hello"
  old_attribute = "old val"
  list_attr     = ["hello"]
  map_attr = {
    foo = "bar"
    test = dependency.vpc.outputs.new_attribute
  }
}
