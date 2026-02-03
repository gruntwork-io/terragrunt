terraform {
  source = "."
}

# This exclude block does NOT specify no_run, so it should default to no_run = false
# The unit should still run in single-unit mode (no early exit)
exclude {
  if      = true
  actions = ["plan"]
}
