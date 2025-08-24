output "dev_lambda_function_url" {
  description = "URL of the Lambda function"
  value       = module.dev.lambda_function_url
}

output "dev_s3_bucket_name" {
  description = "Name of the S3 bucket for static assets"
  value       = module.dev.s3_bucket_name
}

output "prod_lambda_function_url" {
  description = "URL of the Lambda function"
  value       = module.prod.lambda_function_url
}

output "prod_s3_bucket_name" {
  description = "Name of the S3 bucket for static assets"
  value       = module.prod.s3_bucket_name
}
