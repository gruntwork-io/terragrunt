include {
  path = find_in_parent_folders()
}

terraform {
  source = "../../base-module"
}

inputs = {
  person = "Ernie"
}

