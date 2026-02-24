terraform {
  source = "."
}

# Excluded only for plan action
exclude {
  if      = true
  actions = ["plan"]
}
