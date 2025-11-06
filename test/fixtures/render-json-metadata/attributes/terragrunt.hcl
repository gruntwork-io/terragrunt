inputs = {
  region = local.aws_region
  name   = "${local.aws_region}-bucket"
}

locals {
  aws_region = "us-east-1"
}

download_dir = "/tmp"

prevent_destroy = true

exclude {
  if = true
  actions = ["all"]
  no_run = true
}

iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
iam_assume_role_duration = 666

terraform_binary = get_env("TG_TF_PATH", "tofu")
terraform_version_constraint = ">= 0.11"

errors {
  retry "custom_errors" {
    retryable_errors = [
      "(?s).*Error installing provider.*tcp.*connection reset by peer.*",
      "(?s).*ssh_exchange_identification.*Connection closed by remote host.*"
    ]
    max_attempts = 3
    sleep_interval_sec = 5
  }
}

iam_assume_role_session_name = "qwe"

