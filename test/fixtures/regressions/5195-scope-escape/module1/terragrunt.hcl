# This module depends on bastion - it should NOT be discovered when running from bastion/
dependency "bastion" {
  config_path = "../bastion"
}
