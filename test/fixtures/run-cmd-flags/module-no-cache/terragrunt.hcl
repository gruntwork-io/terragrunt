locals {
  scripts_dir = "${get_terragrunt_dir()}/../scripts"
  first       = run_cmd("--terragrunt-no-cache", "${local.scripts_dir}/no_cache_counter.sh")
  second      = run_cmd("--terragrunt-no-cache", "${local.scripts_dir}/no_cache_counter.sh")
}

inputs = {
  first_value  = local.first
  second_value = local.second
}
