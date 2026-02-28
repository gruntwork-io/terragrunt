locals {
  condition = false
  result    = local.condition ? run_cmd("__nonexistent_terragrunt_test_command__") : run_cmd("echo", "branch_false")
}

inputs = {
  result = local.result
}
