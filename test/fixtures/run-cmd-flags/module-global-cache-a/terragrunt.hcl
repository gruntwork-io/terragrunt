locals {
  scripts_dir = "${get_terragrunt_dir()}/../scripts"
  cached      = run_cmd("--terragrunt-global-cache", "${local.scripts_dir}/global_counter.sh")
}

inputs = {
  cached_value = local.cached
}
