locals {
	version = "v0.68.4"
}

unit "app1" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=${local.version}"
	path   = "app1"
}

unit "app2" {
	source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=${local.version}"
	path   = "app2"
}

