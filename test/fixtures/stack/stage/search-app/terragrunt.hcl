include {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../vpc", "../redis"]
}

