terraform {
  backend "s3" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

# Create an arbitrary local resource
resource "null_resource" "text" {
  provisioner "local-exec" {
    command = "echo '[I am a kms-master-key template.]'"
  }
}

output "text" {
  value = "[I am a kms-master-key template.]"
}
