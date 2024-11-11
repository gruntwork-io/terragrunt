locals {
	shared = jsondecode(file(find_in_parent_folders("shared.json")))
}

inputs = {
	shared = local.shared
}
