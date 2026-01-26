terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "~> 6.0"
    }
    google = {
      source  = "registry.opentofu.org/hashicorp/google"
      version = "~> 6.0"
    }
    azurerm = {
      source  = "registry.opentofu.org/hashicorp/azurerm"
      version = "~> 4.0"
    }
    kubernetes = {
      source  = "registry.opentofu.org/hashicorp/kubernetes"
      version = "~> 2.0"
    }
    helm = {
      source  = "registry.opentofu.org/hashicorp/helm"
      version = "~> 3.0"
    }
    vault = {
      source  = "registry.opentofu.org/hashicorp/vault"
      version = "~> 5.0"
    }
    consul = {
      source  = "registry.opentofu.org/hashicorp/consul"
      version = "~> 2.0"
    }
    nomad = {
      source  = "registry.opentofu.org/hashicorp/nomad"
      version = "~> 2.0"
    }
    datadog = {
      source  = "registry.opentofu.org/DataDog/datadog"
      version = "~> 3.0"
    }
    github = {
      source  = "registry.opentofu.org/integrations/github"
      version = "~> 6.0"
    }
    tls = {
      source  = "registry.opentofu.org/hashicorp/tls"
      version = "~> 4.0"
    }
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "~> 3.0"
    }
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.0"
    }
  }
}
