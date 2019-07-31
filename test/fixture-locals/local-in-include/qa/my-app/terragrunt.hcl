locals {
  parent_path = find_in_parent_folders()
}

include {
  path = local.parent_path
}
