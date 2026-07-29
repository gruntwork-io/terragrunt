terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "6.28.0"
    }
    google = {
      source  = "registry.opentofu.org/hashicorp/google"
      version = "6.50.0"
    }
    azurerm = {
      source  = "registry.opentofu.org/hashicorp/azurerm"
      version = "4.58.0"
    }
    kubernetes = {
      source  = "registry.opentofu.org/hashicorp/kubernetes"
      version = "2.38.0"
    }
    helm = {
      source  = "registry.opentofu.org/hashicorp/helm"
      version = "3.1.1"
    }
    vault = {
      source  = "registry.opentofu.org/hashicorp/vault"
      version = "5.6.0"
    }
    consul = {
      source  = "registry.opentofu.org/hashicorp/consul"
      version = "2.22.1"
    }
    nomad = {
      source  = "registry.opentofu.org/hashicorp/nomad"
      version = "2.5.2"
    }
    datadog = {
      source  = "registry.opentofu.org/DataDog/datadog"
      version = "3.85.0"
    }
    github = {
      source  = "registry.opentofu.org/integrations/github"
      version = "6.10.2"
    }
    tls = {
      source  = "registry.opentofu.org/hashicorp/tls"
      version = "4.1.0"
    }
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "3.8.0"
    }
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}
