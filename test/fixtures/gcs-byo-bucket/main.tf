terraform {
  backend "gcs" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

# Create an arbitrary local resource
resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "echo Hello, World!"
  }
}
