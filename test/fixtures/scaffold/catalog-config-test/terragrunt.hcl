# Terragrunt configuration for testing catalog config override behavior
catalog {
  urls = ["../with-shell-and-hooks"]
  no_shell = true
  no_hooks = true
}
