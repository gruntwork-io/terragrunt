terraform {
  source = "."
}

# Excluded with no_run = true - causes early exit in single unit mode
exclude {
  if      = true
  no_run  = true
  actions = ["all"]
}
