terraform {
  source = "../../../modules//base"
}

include {
  path = find_in_parent_folders("root.hcl")
}
