run "valid_string_concat" {
  command = plan
  assert {
    condition     = aws_s3_bucket.bucket.bucket == "tg-test-bucket"
    error_message = "S3 bucket name expected to be tg-test-bucket"
  }
}
