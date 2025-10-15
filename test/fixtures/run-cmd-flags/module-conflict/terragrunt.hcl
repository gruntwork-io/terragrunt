locals {
  scripts_dir = "${get_terragrunt_dir()}/../scripts"
  conflict    = run_cmd("--terragrunt-global-cache", "--terragrunt-no-cache", "${local.scripts_dir}/global_counter.sh")
}

inputs = {
  conflict_value = local.conflict
}
