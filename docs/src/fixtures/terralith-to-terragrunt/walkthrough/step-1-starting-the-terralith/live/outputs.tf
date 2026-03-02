output "lambda_function_url" {
  description = "URL of the Lambda function"
  value       = aws_lambda_function_url.main.function_url
}

output "lambda_function_name" {
  description = "Name of the Lambda function"
  value       = aws_lambda_function.main.function_name
}

output "s3_bucket_name" {
  description = "Name of the S3 bucket for static assets"
  value       = aws_s3_bucket.static_assets.bucket
}

output "s3_bucket_arn" {
  description = "ARN of the S3 bucket for static assets"
  value       = aws_s3_bucket.static_assets.arn
}

output "dynamodb_table_name" {
  description = "Name of the DynamoDB table for asset metadata"
  value       = aws_dynamodb_table.asset_metadata.name
}

output "dynamodb_table_arn" {
  description = "ARN of the DynamoDB table for asset metadata"
  value       = aws_dynamodb_table.asset_metadata.arn
}

output "lambda_role_arn" {
  description = "ARN of the Lambda execution role"
  value       = aws_iam_role.lambda_role.arn
}
