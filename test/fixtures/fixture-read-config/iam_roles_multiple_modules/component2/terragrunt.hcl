iam_role = "arn:aws:iam::${local.aws_id_b}:role/terragrunt"

locals {
  aws_id_b = "component2"
}
