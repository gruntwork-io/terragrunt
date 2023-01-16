terraform {
  required_providers {
    random = {
      version = ">= 3.4.0"
    }
  }

  required_version = ">= 1.2.7"
}

module "terraform_execution_role" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-assumable-role-with-oidc"
  version = "~> 5.4"
}
