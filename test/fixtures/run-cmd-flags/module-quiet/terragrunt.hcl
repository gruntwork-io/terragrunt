locals {
  scripts_dir = "${get_terragrunt_dir()}/../scripts"
  secret      = run_cmd("--terragrunt-quiet", "${local.scripts_dir}/emit_secret.sh")
}

inputs = {
  secret = local.secret
}
