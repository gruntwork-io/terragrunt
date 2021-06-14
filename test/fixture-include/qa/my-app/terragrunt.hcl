include {
  path   = "${find_in_parent_folders()}"
  expose = true
}

inputs = {
  reflect = include.remote_state
}
