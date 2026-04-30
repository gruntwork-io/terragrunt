moved {
  from = module.ddb.aws_dynamodb_table.asset_metadata
  to   = module.prod.module.ddb.aws_dynamodb_table.asset_metadata
}

moved {
  from = module.iam.aws_iam_policy.lambda_basic_execution
  to   = module.prod.module.iam.aws_iam_policy.lambda_basic_execution
}

moved {
  from = module.iam.aws_iam_policy.lambda_dynamodb
  to   = module.prod.module.iam.aws_iam_policy.lambda_dynamodb
}

moved {
  from = module.iam.aws_iam_policy.lambda_s3_read
  to   = module.prod.module.iam.aws_iam_policy.lambda_s3_read
}

moved {
  from = module.iam.aws_iam_role.lambda_role
  to   = module.prod.module.iam.aws_iam_role.lambda_role
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
  to   = module.prod.module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
  to   = module.prod.module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_s3_read
  to   = module.prod.module.iam.aws_iam_role_policy_attachment.lambda_s3_read
}

moved {
  from = module.lambda.aws_lambda_function.main
  to   = module.prod.module.lambda.aws_lambda_function.main
}

moved {
  from = module.lambda.aws_lambda_function_url.main
  to   = module.prod.module.lambda.aws_lambda_function_url.main
}

moved {
  from = module.s3.aws_s3_bucket.static_assets
  to   = module.prod.module.s3.aws_s3_bucket.static_assets
}
