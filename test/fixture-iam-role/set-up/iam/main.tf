provider "aws" {
  region = "us-west-2"
}

data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "assume_role_policy" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "AWS"
      identifiers = [data.aws_caller_identity.current.account_id]
    }
  }
}

resource "aws_iam_role" "terragrunt_test" {
  name               = var.test_role_name
  assume_role_policy = data.aws_iam_policy_document.assume_role_policy.json
}

output "role_terragrunt_test_arn" {
  value = aws_iam_role.terragrunt_test.arn
}

variable "test_role_name" {
  description = "terragrunt test role name"
  default     = ""
}
