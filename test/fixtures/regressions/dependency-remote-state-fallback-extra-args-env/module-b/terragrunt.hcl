terraform {
  extra_arguments "output_env" {
    commands = ["output"]

    env_vars = {
      TG_TEST_REMOTE_STATE_FALLBACK_OUTPUT_VALUE = "argocd"
    }
  }
}

remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    path = "${get_terragrunt_dir()}/${dependency.module_a.outputs.cluster_id}.tfstate"
  }
}

dependency "module_a" {
  config_path                             = "../module-a"
  mock_outputs                            = { cluster_id = "m" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}

inputs = {
  ns = get_terraform_command() == "output" ? get_env("TG_TEST_REMOTE_STATE_FALLBACK_OUTPUT_VALUE") : "argocd"
}
