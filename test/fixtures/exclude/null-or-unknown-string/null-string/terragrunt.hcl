terraform {
  source = "."
}

# Discovery keeps the unit, and the run fails with "null value is not allowed"
exclude {
  if      = tostring(null)
  actions = ["all"]
}
