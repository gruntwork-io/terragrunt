terraform {
  source = "."
}

# Excluded only for apply action
exclude {
  if      = true
  actions = ["apply"]
}
