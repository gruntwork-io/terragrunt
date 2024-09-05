terraform {
  required_providers {
    random = {
      version = ">= 3.4.0"
    }
  }

  required_version = ">= 1.2.7"
}

resource "random_id" "env" {
  byte_length = 8
}

