locals {
	_ = mark_as_read("${get_repo_root()}/marker.txt")
}

unit "ux" {
	source = "${get_repo_root()}/units/u"
	path   = "ux"
}
