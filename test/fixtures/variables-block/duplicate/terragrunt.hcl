include "root" {
  path = find_in_parent_folders("root.hcl")
}

variables {
  kind = "json"

  inputs = {
    from_root = "child"
  }
}
