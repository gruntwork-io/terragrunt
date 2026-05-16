terraform {
  source = "."
}

exclude {
  if      = true
  actions = ["output"]
}
