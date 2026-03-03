variable "name" {
  description = "The name of the IAM role"
  type        = string
}

variable "aws_region" {
  description = "The AWS region to deploy the resources to"
  type        = string
}

variable "s3_bucket_arn" {
  description = "The ARN of the S3 bucket"
  type        = string
}

variable "dynamodb_table_arn" {
  description = "The ARN of the DynamoDB table"
  type        = string
}
