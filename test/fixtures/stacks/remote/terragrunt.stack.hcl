locals {
	version = "v0.6.0"
}

unit "app1" {
	source = "github.com/gruntwork-io/terraform-google-sql.git//modules/cloud-sql?ref=${local.version}"
	path   = "app1"
}

unit "app2" {
	source = "github.com/gruntwork-io/terraform-google-sql.git//modules/cloud-sql?ref=${local.version}"
	path   = "app2"
}

