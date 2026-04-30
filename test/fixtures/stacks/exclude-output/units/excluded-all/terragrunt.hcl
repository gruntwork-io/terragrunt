terraform {
  source = "."
}

exclude {
  if      = true
  no_run  = true
  actions = ["all"]
}
