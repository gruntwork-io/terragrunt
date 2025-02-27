unit "api" {
	source = "../../units/api"
	path   = "api"
	values = {
		ver = "dev-api 1.0.0"
	}
}

unit "db" {
	source = "../../units/db"
	path   = "db"
	values = {
		ver = "dev-db 1.0.0"
	}
}

unit "web" {
	source = "../../units/web"
	path   = "web"
	values = {
		ver = "dev-web 1.0.0"
	}
}
