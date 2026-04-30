terraform {
  source = "."
}

# Excluded but no_run = false - unit runs in single mode, excluded in run --all
exclude {
  if      = true
  no_run  = false
  actions = ["plan", "apply"]
}
