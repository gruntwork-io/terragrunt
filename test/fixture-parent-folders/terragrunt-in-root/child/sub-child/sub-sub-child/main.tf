terraform {
  backend "s3" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

# Create an arbitrary local resource
data "external" "test" {
  program = ["jq", "-n", "{\"greeting\": \"Hello, I am a template.\"}"]
}

