# Root config that is included by child units
# This tests that run_cmd output from included files is visible in stack runs

terraform {
  source = "."
}

locals {
  scripts_dir = "${get_terragrunt_dir()}/../scripts"
  # This run_cmd should emit output that is visible during stack runs
  marker      = run_cmd("${local.scripts_dir}/emit_output.sh")
}

inputs = {
  marker = local.marker
}
