locals {
  test = run_cmd("--terragrunt-quiet", "bash", "-c", "exit 1")
}

terraform {
  source = "."
}

