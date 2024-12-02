include {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../vpc", "../../mgmt/bastion-host", "../backend-app"]
}

