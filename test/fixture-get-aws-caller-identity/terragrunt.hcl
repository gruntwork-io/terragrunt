terraform {

  inputs = {
    account = get_aws_account_id()
    arn = get_aws_account_arn()
    user_id = get_aws_account_user_id()
  }
}
