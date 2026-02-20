locals {
  condition = false
  result    = local.condition ? run_cmd("echo", "branch_true") : run_cmd("echo", "branch_false")
}

inputs = {
  result = local.result
}
