terraform {
  source = "."
}

# The unit declares its own same-name dependency with a unit-side mock value. The autoinclude declares
# dependency "x" too; under shallow merge the autoinclude wins by name and replaces this whole block, so
# dependency.x.outputs.v must resolve to the autoinclude's mock, not "from-unit".
dependency "x" {
  config_path = "../x"

  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs = {
    v = "from-unit"
  }
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.x.outputs.v}.tfstate"
  }
}
