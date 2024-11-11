locals {
	filename = find_in_parent_folders("shared.json")
}

inputs = {
	filename = local.filename
}
