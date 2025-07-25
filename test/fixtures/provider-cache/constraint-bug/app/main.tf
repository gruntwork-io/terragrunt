terraform {
  required_version = "~> 1.0"
  required_providers {

    aws = {
      source  = "hashicorp/aws"
      version = "= 5.100.0"
    }
  }
}

data "aws_caller_identity" "c" {}
