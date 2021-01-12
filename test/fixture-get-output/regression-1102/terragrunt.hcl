remote_state {
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  backend = "swift"
  config = {
    container         = "terraform_state"
    archive_container = "terraform_state-archive"
    cloud             = "dummy"
    region_name       = "dummy"
    state_name        = "dummy"
    user_name         = "dummy"
    user_id           = "dummy"
    tenant_name       = "dummy"
  }
}
