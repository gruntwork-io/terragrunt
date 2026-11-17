locals {
  ignored = run_cmd("echo", "default_only_hcl_should_not_run")
}
