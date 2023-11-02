remote_state {
  backend = "gcs"

  config = {
    project                     = "__FILL_IN_PROJECT__"
    location                    = "__FILL_IN_LOCATION__"
    bucket                      = "__FILL_IN_BUCKET_NAME__"
    prefix                      = "terraform.tfstate"
    credentials                 = "application_default_credentials.json"

    gcs_bucket_labels = {
      owner = "terragrunt_test"
      name  = "terraform_state_storage"
    }
  }
}
