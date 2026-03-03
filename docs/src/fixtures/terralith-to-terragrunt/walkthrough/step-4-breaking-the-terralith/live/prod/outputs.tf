output "lambda_function_url" {
  description = "URL of the Lambda function"
  value       = module.main.lambda_function_url
}

output "s3_bucket_name" {
  description = "Name of the S3 bucket for static assets"
  value       = module.main.s3_bucket_name
}
