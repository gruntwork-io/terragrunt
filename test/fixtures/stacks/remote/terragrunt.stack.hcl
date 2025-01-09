locals {
	version = "2d526f9ce8f295d5754e51856999a5cc35f26c7e"
}

unit "app1" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app1"
}

unit "app2" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/basic/units/chick?ref=${local.version}"
	path   = "app2"
}

