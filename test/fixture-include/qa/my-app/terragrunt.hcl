include {
  path = "${find_in_parent_folders()}"
}

inputs = {
  reflect = include.remote_state
}
