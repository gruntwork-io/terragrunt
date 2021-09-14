iam_role = "__IAM_PLACEHOLDER__"

locals {
  iam_text = run_cmd("echo", "${get_aws_account_id()}")
}
