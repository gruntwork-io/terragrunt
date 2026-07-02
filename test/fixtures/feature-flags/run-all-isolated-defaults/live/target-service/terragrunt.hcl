feature "toggle" {
  default = true
}

terraform {
  source = "${find_in_parent_folders("catalog")}//echo-toggle"
}

inputs = {
  effective_toggle = feature.toggle.value
  raw_toggle       = true
}
