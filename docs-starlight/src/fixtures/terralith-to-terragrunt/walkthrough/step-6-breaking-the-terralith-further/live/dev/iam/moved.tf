moved {
  from = module.iam.aws_iam_policy.lambda_basic_execution
  to   = aws_iam_policy.lambda_basic_execution
}

moved {
  from = module.iam.aws_iam_policy.lambda_dynamodb
  to   = aws_iam_policy.lambda_dynamodb
}

moved {
  from = module.iam.aws_iam_policy.lambda_s3_read
  to   = aws_iam_policy.lambda_s3_read
}

moved {
  from = module.iam.aws_iam_role.lambda_role
  to   = aws_iam_role.lambda_role
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
  to   = aws_iam_role_policy_attachment.lambda_basic_execution
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
  to   = aws_iam_role_policy_attachment.lambda_dynamodb
}

moved {
  from = module.iam.aws_iam_role_policy_attachment.lambda_s3_read
  to   = aws_iam_role_policy_attachment.lambda_s3_read
}
