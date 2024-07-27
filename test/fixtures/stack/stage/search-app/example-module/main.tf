terraform {
  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

# Create an arbitrary local resource
data "external" "text" {
  program = ["jq", "-n", "{\"text\": \"Example text from a module\"}"]
}

output "text" {
  value = data.external.text.result.text
}
