terraform {
  source = "."
}

exclude {
  if      = true
  actions = ["plan", "apply", "destroy", "output"]
}
