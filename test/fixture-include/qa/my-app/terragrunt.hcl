include "root" {
  path   = "${find_in_parent_folders()}"
  expose = true

  # Don't merge in remote state block so we store state locally
  merge_strategy = "no_merge"
}

inputs = {
  reflect = include.root.remote_state
}
