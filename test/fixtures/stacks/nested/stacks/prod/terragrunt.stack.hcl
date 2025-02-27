unit "api" {
	source = "../../units/api"
	path   = "api"
	values = {
		ver = "prod-api 1.0.0"
	}
}

unit "db" {
	source = "../../units/db"
	path   = "db"
	values = {
		ver = "prod-db 1.0.0"
	}
}

unit "web" {
	source = "../../units/web"
	path   = "web"
	values = {
		ver = "prod-web 1.0.0"
	}
}
