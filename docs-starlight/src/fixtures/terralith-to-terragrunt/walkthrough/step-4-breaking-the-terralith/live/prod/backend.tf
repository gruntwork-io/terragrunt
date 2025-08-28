terraform {
  backend "s3" {
    bucket       = "terragrunt-to-terralith-blog-2025-07-31-01"
    key          = "prod/tofu.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
