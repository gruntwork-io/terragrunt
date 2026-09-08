terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "5.11.0"
    }
  }
  required_version = ">= 1.2.7"
}

provider "aws" {
  region     = "us-east-1"
  access_key = "mock-access-key"
  secret_key = "mock-secret-key"

  # This fixture is only ever planned, never applied. Without the mock
  # credentials and these skips, configuring the provider calls STS
  # GetCallerIdentity, which would make the tflint tests need real AWS
  # credentials to lint static HCL.
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
}

resource "aws_s3_bucket" "bucket" {
  bucket = var.bucket_name

}
