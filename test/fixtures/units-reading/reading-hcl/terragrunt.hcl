locals {
	shared = read_terragrunt_config(find_in_parent_folders("shared.hcl")).locals
}

inputs = {
	shared = local.shared
}
