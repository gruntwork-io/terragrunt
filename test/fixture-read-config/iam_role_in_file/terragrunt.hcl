iam_role = "arn:aws:iam::666666666666:role/terragrunttest"

remote_state {
  backend = "local"
  generate = {
    // state file should load value from iam_role
    path      = "${get_aws_account_id()}.txt"
    if_exists = "overwrite"
  }
  config = {
    path = "terraform.tfstate"
  }
}
