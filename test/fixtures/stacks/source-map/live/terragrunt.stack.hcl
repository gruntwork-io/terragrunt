
unit "app1" {
	source = "git::https://git-host.com/not-existing-repo.git//fixtures/stacks/source-map/units/app"
	path   = "app1"
	values = {
		input = "app1"
	}
}

unit "app2" {
	source = "git::https://git-host.com/not-existing-repo.git//fixtures/stacks/source-map/units/app"
	path   = "app2"
	values = {
		input = "app2"
	}
}
