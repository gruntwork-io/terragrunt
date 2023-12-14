include {
  path = find_in_parent_folders()
}

catalog {
  urls = [
    "terraform-aws-eks",
    "/repo-copier",
    "./terraform-aws-service-catalog",
    "/project/terragrunt/test/terraform-aws-vpc",
    "github.com/gruntwork-io/terraform-aws-lambda",
  ]
}
