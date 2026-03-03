moved {
  from = module.lambda.aws_lambda_function.main
  to   = aws_lambda_function.main
}

moved {
  from = module.lambda.aws_lambda_function_url.main
  to   = aws_lambda_function_url.main
}
