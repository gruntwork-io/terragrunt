terraform {
  source = "."
}

feature "enable_unit" {
  default = false
}

# Excluded based on feature flag with no_run = true
# When enable_unit=false (default), excluded with early exit
# When enable_unit=true, runs normally
exclude {
  if      = !feature.enable_unit.value
  no_run  = true
  actions = ["all"]
}
