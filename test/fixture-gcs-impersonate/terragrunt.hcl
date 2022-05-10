# Configure Terragrunt to automatically store tfstate files in a GCS bucket

locals {
  service_account = run_cmd("bash", "-c","gcloud config get-value account")
}



remote_state {
  backend = "gcs"

  config = {
    project  = "__FILL_IN_PROJECT__"
    location = "__FILL_IN_LOCATION__"
    bucket   = "__FILL_IN_BUCKET_NAME__"
    impersonate_service_account = local.service_account
    prefix   = "terraform.tfstate"

    gcs_bucket_labels = {
      owner = "terragrunt_test"
      name  = "terraform_state_storage"
    }
  }
}
