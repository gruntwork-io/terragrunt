locals {
	project = "test-project"
}

unit "app1" {
	source = "../units/app"
	path   = "app1"

	values = {
		project    = local.project
		deployment = "app1"
	}
}

unit "app2" {
	source = "../units/app"
	path   = "app2"

	values = {
		project    = local.project
		deployment = "app2"
	}
}
