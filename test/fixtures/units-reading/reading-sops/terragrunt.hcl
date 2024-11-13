locals {
	shared = sops_decrypt_file(find_in_parent_folders("secrets.txt"))
}

inputs = {
	shared = local.shared
}
