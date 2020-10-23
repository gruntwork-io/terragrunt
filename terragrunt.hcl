# stage/terragrunt.hcl
remote_state {

  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    bucket = "ana-ina-11115"
    enable_bucket_accesslogging = true
    key = "${path_relative_to_include()}/terraform.tfstate"
    region         = "eu-west-2"
    encrypt        = true
    dynamodb_table = "ana-ina-11115"
  }
}