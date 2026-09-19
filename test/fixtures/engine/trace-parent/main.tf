terraform {
  required_providers {
    external = {
      source  = "registry.opentofu.org/hashicorp/external"
      version = "2.3.5"
    }
  }
}

data "external" "traceparent" {
  program = ["${path.module}/get_traceparent.sh"]

  query = {
    nonce = timestamp()
  }
}

output "traceparent_value" {
  value = data.external.traceparent.result["traceparent"]
}
