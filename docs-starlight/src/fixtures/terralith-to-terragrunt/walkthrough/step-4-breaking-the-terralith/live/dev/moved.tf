moved {
  from = module.dev.module.ddb.aws_dynamodb_table.asset_metadata
  to   = module.main.module.ddb.aws_dynamodb_table.asset_metadata
}

moved {
  from = module.dev.module.iam.aws_iam_policy.lambda_basic_execution
  to   = module.main.module.iam.aws_iam_policy.lambda_basic_execution
}

moved {
  from = module.dev.module.iam.aws_iam_policy.lambda_dynamodb
  to   = module.main.module.iam.aws_iam_policy.lambda_dynamodb
}

moved {
  from = module.dev.module.iam.aws_iam_policy.lambda_s3_read
  to   = module.main.module.iam.aws_iam_policy.lambda_s3_read
}

moved {
  from = module.dev.module.iam.aws_iam_role.lambda_role
  to   = module.main.module.iam.aws_iam_role.lambda_role
}

moved {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
  to   = module.main.module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
}

moved {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
  to   = module.main.module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
}

moved {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_s3_read
  to   = module.main.module.iam.aws_iam_role_policy_attachment.lambda_s3_read
}

moved {
  from = module.dev.module.lambda.aws_lambda_function.main
  to   = module.main.module.lambda.aws_lambda_function.main
}

moved {
  from = module.dev.module.lambda.aws_lambda_function_url.main
  to   = module.main.module.lambda.aws_lambda_function_url.main
}

moved {
  from = module.dev.module.s3.aws_s3_bucket.static_assets
  to   = module.main.module.s3.aws_s3_bucket.static_assets
}
