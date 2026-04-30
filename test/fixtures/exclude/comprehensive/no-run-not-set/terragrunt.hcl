terraform {
  source = "."
}

# Excluded but no_run is not set (defaults to false) - unit runs in single mode
exclude {
  if      = true
  actions = ["plan"]
}
