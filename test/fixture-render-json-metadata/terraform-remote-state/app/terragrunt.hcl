include "terraform" {
  path   = "../common/terraform.hcl"
}

include "remote_state" {
  path   = "../common/remote_state.hcl"
}
