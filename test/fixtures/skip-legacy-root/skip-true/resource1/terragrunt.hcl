include {
  path = find_in_parent_folders("root.hcl")
}

exclude {
  if = false
  actions = ["all"]
  no_run = true
}

inputs = {
  person = "Ernie"
}

