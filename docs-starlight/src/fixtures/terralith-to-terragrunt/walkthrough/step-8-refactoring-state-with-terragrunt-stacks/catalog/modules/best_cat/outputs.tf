output "lambda_function_url" {
  description = "URL of the Lambda function"
  value       = module.lambda.url
}

output "lambda_function_name" {
  description = "Name of the Lambda function"
  value       = module.lambda.name
}

output "s3_bucket_name" {
  description = "Name of the S3 bucket for static assets"
  value       = module.s3.name
}

output "s3_bucket_arn" {
  description = "ARN of the S3 bucket for static assets"
  value       = module.s3.arn
}

output "dynamodb_table_name" {
  description = "Name of the DynamoDB table for asset metadata"
  value       = module.ddb.name
}

output "dynamodb_table_arn" {
  description = "ARN of the DynamoDB table for asset metadata"
  value       = module.ddb.arn
}

output "lambda_role_arn" {
  description = "ARN of the Lambda execution role"
  value       = module.iam.arn
}
