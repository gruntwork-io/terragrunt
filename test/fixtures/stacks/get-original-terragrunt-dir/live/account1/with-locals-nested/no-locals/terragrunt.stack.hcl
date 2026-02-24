stack "units" {
  source = find_in_parent_folders("stacks/with-locals")
  path   = "unit_dirs"
}
