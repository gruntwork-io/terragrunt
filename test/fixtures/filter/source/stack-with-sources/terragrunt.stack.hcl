unit "vpc" {
  source = "github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc"
  path   = "vpc"
}

unit "app" {
  source = "github.com/example/terraform-modules//modules/app"
  path   = "app"
}

stack "database" {
  source = "tfr://registry.terraform.io/terraform-aws-modules/rds/aws"
  path   = "database"
}

