resource "aws_lambda_function" "main" {
  function_name = "${var.name}-function"

  filename         = var.lambda_zip_file
  source_code_hash = filebase64sha256(var.lambda_zip_file)

  role = var.lambda_role_arn

  handler       = var.lambda_handler
  runtime       = var.lambda_runtime
  timeout       = var.lambda_timeout
  memory_size   = var.lambda_memory_size
  architectures = var.lambda_architectures

  environment {
    variables = {
      S3_BUCKET_NAME      = var.s3_bucket_name
      DYNAMODB_TABLE_NAME = var.dynamodb_table_name
    }
  }
}

resource "aws_lambda_function_url" "main" {
  function_name      = aws_lambda_function.main.function_name
  authorization_type = "NONE"
}
