locals {
	shared = read_tfvars_file(find_in_parent_folders("shared.tfvars"))
}

inputs = {
	shared = local.shared
}
