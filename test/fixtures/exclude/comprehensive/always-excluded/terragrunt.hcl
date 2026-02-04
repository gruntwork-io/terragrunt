terraform {
  source = "."
}

# Always excluded for all actions
exclude {
  if      = true
  actions = ["all"]
}
