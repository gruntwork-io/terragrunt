# Project A dependency on Project B using values
dependency "project_b" {
  config_path = "../../../project-B/.terragrunt-stack/${values.env}"

  mock_outputs = {
    project_b_output = "mock-project-b-output"
  }

  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan"]
}

terraform {
  source = "."
}

# This will fail parsing if module/ isn't excluded from discovery
# because ${values} only exists in generated stack context, not during standalone parsing
inputs = {
  environment = values.env
}

