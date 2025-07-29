terraform {
  source = "."
}

feature "enable_unit" {
  default = false
}

exclude {
  if      = !feature.enable_unit.value
  no_run  = true
  actions = ["all"]
}
