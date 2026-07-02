locals {
	version = "main"
}

unit "app1" {
	source = "git::__MIRROR_URL__//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app1"
}

unit "app2" {
	source = "git::__MIRROR_URL__//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app2"
}

