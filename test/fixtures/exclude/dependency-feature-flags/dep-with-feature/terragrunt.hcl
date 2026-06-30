feature "skip_ci" {
  default = false
}

terraform {
  source = "."
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
