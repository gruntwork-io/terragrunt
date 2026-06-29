terraform {
  source = "${get_terragrunt_dir()}/module"

  before_hook "use_dep" {
    commands = ["init", "apply", "plan"]
    execute  = ["echo", "${dependency.upstream.outputs.cluster_id}"]
  }
}

dependency "upstream" {
  config_path                             = "../upstream"
  mock_outputs                            = { cluster_id = "m" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}
