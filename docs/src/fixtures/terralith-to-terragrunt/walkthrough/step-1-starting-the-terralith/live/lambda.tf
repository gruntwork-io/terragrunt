resource "aws_lambda_function" "main" {
  function_name = "${var.name}-function"

  filename         = var.lambda_zip_file
  source_code_hash = filebase64sha256(var.lambda_zip_file)

  role = aws_iam_role.lambda_role.arn

  handler       = var.lambda_handler
  runtime       = var.lambda_runtime
  timeout       = var.lambda_timeout
  memory_size   = var.lambda_memory_size
  architectures = var.lambda_architectures

  environment {
    variables = {
      S3_BUCKET_NAME      = aws_s3_bucket.static_assets.bucket
      DYNAMODB_TABLE_NAME = aws_dynamodb_table.asset_metadata.name
    }
  }

  depends_on = [
    aws_iam_role_policy_attachment.lambda_s3_read,
    aws_iam_role_policy_attachment.lambda_dynamodb,
    aws_iam_role_policy_attachment.lambda_basic_execution
  ]
}

resource "aws_lambda_function_url" "main" {
  function_name      = aws_lambda_function.main.function_name
  authorization_type = "NONE"
}
