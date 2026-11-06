locals {
  ignored = run_cmd("echo", "app_default_hcl_should_not_run")
}
