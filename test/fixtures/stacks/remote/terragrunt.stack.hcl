locals {
	version = "main"
}

unit "app1" {
	source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}&depth=1"
	path   = "app1"
}

unit "app2" {
	source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}&depth=1"
	path   = "app2"
}

