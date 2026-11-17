locals {
  region_hcl = find_in_parent_folders("region.hcl")
  region     = read_terragrunt_config(local.region_hcl).locals.region
}

remote_state {
  backend = "gcs"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    project  = "__FILL_IN_PROJECT_ID__"
    location = "us-central1"
    bucket   = "__FILL_IN_BUCKET_NAME__"

    prefix = path_relative_to_include()
  }
}

generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "google" {
  project = "__FILL_IN_PROJECT_ID__"
  region  = "${local.region}"
}
EOF
}

inputs = {
  project_id = "__FILL_IN_PROJECT_ID__"
}
