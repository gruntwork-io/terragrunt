terraform {
  backend "s3" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

data "external" "hello" {
  program = ["bash", "-c", "sample_var=\"$$(cat - jq '.sample_var')\" && echo '{\"output\": \"Hello, I am a template. My sample_var value = $$sample_var\"}'"]

  query = {
    sample_var = var.sample_var
  }
}

