terraform {
  before_hook "use_dep" {
    commands = ["init", "apply", "plan"]
    execute  = ["echo", "${dependency.module_a.outputs.cluster_id}"]
  }
}

dependency "module_a" {
  config_path                             = "../module-a"
  mock_outputs                            = { cluster_id = "m" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}
