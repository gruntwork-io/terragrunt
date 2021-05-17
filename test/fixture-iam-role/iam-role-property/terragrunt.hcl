remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = "foo.tfstate"
  }
}

iam_role = "__IAM_ROLE_ARN__"

inputs = {
  get_caller_identity_arn  = run_cmd("aws", "sts", "get-caller-identity", "--query", "Arn")
}
