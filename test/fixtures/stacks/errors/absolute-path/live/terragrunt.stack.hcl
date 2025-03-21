
unit "app1" {
	source = "../units/app"
	path   = "${get_repo_root()}/app1"
	values = {
		input = "app1"
	}
}

unit "app2" {
	source = "../units/app"
	path   = "${get_repo_root()}/app2"
	values = {
		input = "app2"
	}
}

unit "app3" {
	source = "../units/app"
	path   = "app3"
	values = {
		input = "app3"
	}
}
