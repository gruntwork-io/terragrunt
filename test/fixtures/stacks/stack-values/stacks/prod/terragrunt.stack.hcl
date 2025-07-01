unit "prod-app-1" {
	source = "${get_repo_root()}/units/app"
	path   = "prod-app-1"
	values = {
		project = values.project
		env = values.env
		data = "prod-app-1"
	}
}

unit "prod-app-2" {
	source = "${get_repo_root()}/units/app"
	path   = "prod-app-2"
	values = {
		project = values.project
		env = values.env
		data = "prod-app-2"
	}
}
