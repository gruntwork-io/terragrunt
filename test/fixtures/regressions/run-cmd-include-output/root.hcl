# Root config that is included by child units
# This tests that run_cmd output from included files is visible in stack runs

terraform {
  source = "."
}

locals {
  # Use the directory containing root.hcl (found via find_in_parent_folders) to construct
  # a consistent path regardless of which unit includes this file.
  root_dir = dirname(find_in_parent_folders("root.hcl"))
  # This run_cmd should emit output that is visible during stack runs.
  # We use --terragrunt-global-cache to ensure both units share the same cache entry.
  marker = run_cmd("--terragrunt-global-cache", "${local.root_dir}/scripts/emit_output.sh")
}

inputs = {
  marker = local.marker
}
