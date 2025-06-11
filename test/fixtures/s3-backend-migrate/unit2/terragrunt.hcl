terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

remote_state {
  backend = "gcs"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    prefix   = "unit2/tofu.tfstate"
    location = "__FILL_IN_LOCATION__"
    project  = "__FILL_IN_PROJECT__"
    bucket   = "__FILL_IN_BUCKET_NAME__"
  }
}
