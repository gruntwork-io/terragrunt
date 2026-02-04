terraform {
  source = "."
}

# Excluded for all actions except output
exclude {
  if      = true
  actions = ["all_except_output"]
}
