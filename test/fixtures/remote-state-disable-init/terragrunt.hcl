// Test fixture for Issue #1422: remote_state.disable_init behavior
remote_state {
  backend = "local"
  disable_init = true
  config = {
    path = "test.tfstate"
  }
}
