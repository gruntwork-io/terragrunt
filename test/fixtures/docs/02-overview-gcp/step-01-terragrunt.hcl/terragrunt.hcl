# Configure the remote backend
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

    prefix = "tofu"
  }
}

# Configure the Google provider
generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "google" {
  project = "__FILL_IN_PROJECT_ID__"
  region  = "us-central1"
}
EOF
}

# Configure the module
#
# The `tfr:///` shorthand uses the default registry host for OpenTofu or Terraform.
# Set `TG_TF_DEFAULT_REGISTRY_HOST` to use a different host.
#
# The module lives at:
# https://search.opentofu.org/module/terraform-google-modules/network/google/latest
#
# The shorthand needs three slashes after `tfr:`, as in `tfr:///`.
terraform {
  source = "tfr:///terraform-google-modules/network/google?version=18.3.1"
}

# Configure the inputs for the module
inputs = {
  project_id   = "__FILL_IN_PROJECT_ID__"
  network_name = "__FILL_IN_NAME_PREFIX__-vpc-us-central1"

  subnets = [
    {
      subnet_name   = "private"
      subnet_ip     = "10.0.1.0/24"
      subnet_region = "us-central1"
    },
    {
      subnet_name   = "public"
      subnet_ip     = "10.0.101.0/24"
      subnet_region = "us-central1"
    },
  ]
}
