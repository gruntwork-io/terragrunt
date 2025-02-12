terraform {
  source = "test"
}

include {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../../module-a", "../../module-b/module-b-child"]
}
