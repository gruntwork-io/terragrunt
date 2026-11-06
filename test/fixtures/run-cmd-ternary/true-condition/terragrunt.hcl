locals {
  condition = true
  result    = local.condition ? run_cmd("echo", "branch_true") : run_cmd("__nonexistent_terragrunt_test_command__")
}

inputs = {
  result = local.result
}
