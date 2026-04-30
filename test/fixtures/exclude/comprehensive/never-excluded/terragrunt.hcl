terraform {
  source = "."
}

# Never excluded (if = false)
exclude {
  if      = false
  actions = ["all"]
}
