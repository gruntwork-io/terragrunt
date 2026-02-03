terraform {
  source = "."
}

# This should allow the unit to run since no_run is explicitly false
exclude {
  if      = true
  no_run  = false
  actions = ["plan", "apply"]
}
