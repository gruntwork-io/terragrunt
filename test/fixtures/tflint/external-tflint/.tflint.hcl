plugin "terraform" {
  enabled = true
  version = "0.2.1"
  source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}

plugin "aws" {
  enabled = true
  version = "0.25.0"
  source  = "github.com/terraform-linters/tflint-ruleset-aws"
}

config {
  module = true
}

rule "aws_s3_bucket_name" {
  enabled = true
  regex = "my-prefix-.*"
  prefix = "my-prefix-"
}
