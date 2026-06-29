terraform {
  source = "${get_terragrunt_dir()}/module"
}

dependency "middle" {
  config_path                             = "../middle"
  mock_outputs                            = { ns = "argocd" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}

inputs = {
  ns = dependency.middle.outputs.ns
}
