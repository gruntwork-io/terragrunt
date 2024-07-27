terraform {
  backend "gcs" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

data "external" "hello" {
  program = ["jq", "-n", "--arg", "name", var.sample_var, "{\"greeting\": $name}"]
}

