generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite"
  contents = <<EOF

provider "aws" {
  region  = "ca-central-1"
}

terraform {
  backend "s3" {
    encrypt = true
    bucket = "test-bucket-666"
    dynamodb_table = "test-666"
    region = "ca-central-1"
    key = "terraform.tfstate"
  }
}

EOF
}
