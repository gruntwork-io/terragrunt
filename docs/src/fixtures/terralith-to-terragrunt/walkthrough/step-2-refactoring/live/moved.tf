moved {
  from = aws_dynamodb_table.asset_metadata
  to   = module.ddb.aws_dynamodb_table.asset_metadata
}

moved {
  from = aws_iam_policy.lambda_basic_execution
  to   = module.iam.aws_iam_policy.lambda_basic_execution
}

moved {
  from = aws_iam_policy.lambda_dynamodb
  to   = module.iam.aws_iam_policy.lambda_dynamodb
}

moved {
  from = aws_iam_policy.lambda_s3_read
  to   = module.iam.aws_iam_policy.lambda_s3_read
}

moved {
  from = aws_iam_role.lambda_role
  to   = module.iam.aws_iam_role.lambda_role
}

moved {
  from = aws_iam_role_policy_attachment.lambda_basic_execution
  to   = module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
}

moved {
  from = aws_iam_role_policy_attachment.lambda_dynamodb
  to   = module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
}

moved {
  from = aws_iam_role_policy_attachment.lambda_s3_read
  to   = module.iam.aws_iam_role_policy_attachment.lambda_s3_read
}

moved {
  from = aws_lambda_function.main
  to   = module.lambda.aws_lambda_function.main
}

moved {
  from = aws_lambda_function_url.main
  to   = module.lambda.aws_lambda_function_url.main
}

moved {
  from = aws_s3_bucket.static_assets
  to   = module.s3.aws_s3_bucket.static_assets
}
