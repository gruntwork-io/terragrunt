unit "app_3" {
	source = "${get_repo_root()}/units/app"
	path   = "app_3"
	values = {
		data = "app_3"
	}
}

unit "app_4" {
	source = "${get_repo_root()}/units/app"
	path   = "app_4"
	values = {
		data = "app_4"
	}
}

