terraform {
  required_providers {
    direct = {
      source  = "hashicorp/null"
    }
    network = {
      source  = "example.com/network/null"
      version = "3.2.2"
    }
    filesystem = {
      source  = "example.com/filesystem/null"
      version = "3.2.2"
    }
  }
}

provider "null" {
  # Configuration options
}
