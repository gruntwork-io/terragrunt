include "root" {
  path   = "${find_in_parent_folders()}"
  expose = true
}

inputs = {
  reflect = include.root.remote_state
}
