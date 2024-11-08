inputs = {
	shared_hcl    = read_terragrunt_config(find_in_parent_folders("shared.hcl")).locals
	shared_tfvars = read_tfvars_file(find_in_parent_folders("shared.tfvars"))
}
