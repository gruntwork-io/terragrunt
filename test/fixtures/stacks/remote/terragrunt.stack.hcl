locals {
	version = "main"
}

unit "app1" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app1"
}

unit "app2" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app2"
}

