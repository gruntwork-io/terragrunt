terraform {
  extra_arguments "use_dep" {
    commands = ["init", "plan", "apply"]
    env_vars = {
      CLUSTER_ID = "${dependency.module_a.outputs.cluster_id}"
    }
    arguments = ["-var", "foo=${dependency.module_a.outputs.cluster_id}"]
  }
}

dependency "module_a" {
  config_path                             = "../module-a"
  mock_outputs                            = { cluster_id = "m" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}
