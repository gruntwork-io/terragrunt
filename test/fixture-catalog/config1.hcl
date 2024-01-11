locals {
  baseRepo = "github.com/gruntwork-io"
}

catalog {
  urls = [
    "terraform-aws-eks",
    "/repo-copier",
    "./terraform-aws-service-catalog",
    "/project/terragrunt/test/terraform-aws-vpc",
    "${local.baseRepo}/terraform-aws-lambda",
  ]
}
