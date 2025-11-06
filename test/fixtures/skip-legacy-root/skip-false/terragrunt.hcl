exclude {
  if = false
  actions = ["all"]
  no_run = true
}

terraform {
  source = "../base-module"
}

inputs = {
  person = "Hobbs"
}

