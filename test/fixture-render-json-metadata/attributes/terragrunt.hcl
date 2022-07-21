inputs = {
  region = local.aws_region
  name   = "${local.aws_region}-bucket"
}

locals {
  aws_region = "us-east-1"
}

download_dir = "/tmp"

prevent_destroy = true
skip = true

iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
iam_assume_role_duration = 666

terraform_binary = "terraform"
terraform_version_constraint = ">= 0.11"

retryable_errors = [
  "(?s).*Error installing provider.*tcp.*connection reset by peer.*",
  "(?s).*ssh_exchange_identification.*Connection closed by remote host.*"
]
iam_assume_role_session_name = "qwe"

