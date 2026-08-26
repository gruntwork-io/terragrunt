dependency "module_b" {
  config_path                             = "../module-b"
  mock_outputs                            = { ns = "argocd" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}

inputs = {
  ns = dependency.module_b.outputs.ns
}
