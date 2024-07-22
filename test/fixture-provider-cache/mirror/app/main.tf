terraform {
  required_providers {
    null = {
      source = "hashicorp/null"
    }
    network = {
      source  = "example.com/network/network"
      version = "3.2.2"
    }
    filesystem = {
      source  = "example.com/filesystem/filesystem"
      version = "3.2.2"
    }
  }
}

provider "null" {
  # Configuration options
}
