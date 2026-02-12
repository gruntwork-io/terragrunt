terraform {
  source = "."
}

# Excluded with no_run=true but only for plan action
# When running apply, this unit should run (action doesn't match)
exclude {
  if      = true
  no_run  = true
  actions = ["plan"]
}
