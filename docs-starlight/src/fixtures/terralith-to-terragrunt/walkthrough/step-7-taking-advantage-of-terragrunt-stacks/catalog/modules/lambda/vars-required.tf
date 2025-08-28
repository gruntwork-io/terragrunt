variable "name" {
  description = "Name used for all resources"
  type        = string
}

variable "aws_region" {
  description = "AWS region to deploy the resources to"
  type        = string
}

variable "lambda_zip_file" {
  description = "Path to the Lambda function zip file"
  type        = string
}

variable "lambda_role_arn" {
  description = "Lambda function role ARN"
  type        = string
}

variable "s3_bucket_name" {
  description = "S3 bucket name"
  type        = string
}

variable "dynamodb_table_name" {
  description = "DynamoDB table name"
  type        = string
}
