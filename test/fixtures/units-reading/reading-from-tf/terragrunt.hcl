locals {
	filename = mark_as_read(find_in_parent_folders("shared.json"))
}

inputs = {
	filename = local.filename
}
