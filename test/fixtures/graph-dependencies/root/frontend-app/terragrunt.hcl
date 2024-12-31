include {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../backend-app", "../vpc"]
}
