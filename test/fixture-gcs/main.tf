terraform {
  backend "gcs" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

# Create an arbitrary local resource
data "external" "test" {
  program = ["jq", "-n", "--arg", "sample", var.sample_var, "{\"test\": \"Hello, I am a template. My sample_var value = \\($sample)\"}"]
}

