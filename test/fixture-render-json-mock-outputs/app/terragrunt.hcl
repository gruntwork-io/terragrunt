include "root" {
  path = find_in_parent_folders()
}

dependency "module" {
  config_path = "../dependency"

  mock_outputs = {
    security_group_id = "sg-abcd1234"
    bastion_host_security_group_id = "123"
  }
  mock_outputs_allowed_terraform_commands = ["validate" ]
}
