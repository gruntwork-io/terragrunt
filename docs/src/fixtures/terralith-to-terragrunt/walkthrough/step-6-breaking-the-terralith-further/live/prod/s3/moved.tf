moved {
  from = module.s3.aws_s3_bucket.static_assets
  to   = aws_s3_bucket.static_assets
}
