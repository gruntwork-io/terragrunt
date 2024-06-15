terraform {
  required_providers {
    null = {
      source  = "example.com/hashicorp/null"
      version = "3.2.2"
    }
  }
}

provider "null" {
  # Configuration options
}
