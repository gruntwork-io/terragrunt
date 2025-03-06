unit "dev-app-1" {
	source = "${get_repo_root()}/units/app"
	path   = "dev-app-1"
	values = {
		project = values.project
		env = values.env
		data = "dev-app-1"
	}
}

unit "dev-app-2" {
	source = "${get_repo_root()}/units/app"
	path   = "dev-app-2"
	values = {
		project = values.project
		env = values.env
		data = "dev-app-2"
	}
}
