terraform {
  source = "."
}

# find and list keep the unit, and run --all fails with "null value is not allowed" before any unit runs
exclude {
  if      = tostring(null)
  actions = ["all"]
}
