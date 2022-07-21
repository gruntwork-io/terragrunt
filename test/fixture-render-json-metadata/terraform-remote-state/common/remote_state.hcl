remote_state {
  backend = "s3"
  config = {
    bucket = "mybucket"
    key    = "path/to/my/key"
    region = "us-east-1"
  }
}