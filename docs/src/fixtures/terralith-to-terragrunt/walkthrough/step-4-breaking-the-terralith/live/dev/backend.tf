terraform {
  backend "s3" {
    bucket       = "terragrunt-to-terralith-tfstate-2025-09-24-2359"
    key          = "dev/tofu.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
