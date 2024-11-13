locals {
	shared = jsondecode(file(mark_as_read(find_in_parent_folders("shared.json"))))
}

inputs = {
	shared = local.shared
}
