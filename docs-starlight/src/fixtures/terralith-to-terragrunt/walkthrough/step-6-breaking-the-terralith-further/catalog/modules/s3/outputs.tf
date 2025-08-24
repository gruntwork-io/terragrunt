output "name" {
  value = aws_s3_bucket.static_assets.bucket
}

output "arn" {
  value = aws_s3_bucket.static_assets.arn
}
