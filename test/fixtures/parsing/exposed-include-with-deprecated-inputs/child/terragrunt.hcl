# Child configuration that includes compcommon with expose = true
# This should trigger the bug where the deprecated syntax in compcommon isn't detected

include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
}

include "compcommon" {
  path = find_in_parent_folders("compcommon.hcl")
  expose = true
}

terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixtures/download/hello-world"
}

dependency "service" {
  config_path = "../dep"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs = {
    some_value = "mock-service-value"
  }
  mock_outputs_merge_strategy_with_state = "shallow"
}

# Reference the exposed include - this will try to evaluate compcommon
# which contains the deprecated syntax
inputs = {
  from_common = try(include.compcommon.inputs.value_from_dep, "fallback")
  from_service = dependency.service.outputs.some_value
}
