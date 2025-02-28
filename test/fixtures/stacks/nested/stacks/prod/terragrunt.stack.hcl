unit "prod-api" {
	source = "${get_repo_root()}/units/api"
	path   = "api"
	values = {
		ver = "prod-api 1.0.0"
	}
}

unit "prod-db" {
	source = "${get_repo_root()}/units/db"
	path   = "db"
	values = {
		ver = "prod-db 1.0.0"
	}
}

unit "prod-web" {
	source = "${get_repo_root()}/units/web"
	path   = "web"
	values = {
		ver = "prod-web 1.0.0"
	}
}
