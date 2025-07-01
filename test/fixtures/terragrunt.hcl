terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    key            = "${path_relative_to_include()}/terraform.tfstate"
    bucket         = "__FILL_IN_BUCKET_NAME__"
    region         = "__FILL_IN_REGION__"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
  }
}
