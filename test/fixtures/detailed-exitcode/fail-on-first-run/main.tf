terraform {
  required_providers {
    external = {
      source  = "registry.opentofu.org/hashicorp/external"
      version = "2.3.5"
    }
  }
}

data "external" "script" {
  program = [
    "/bin/bash",
    "-c",
    <<EOT
      set -euo pipefail
      if [[ -f .file ]]; then
        echo '{}'
        rm .file
      else
        touch .file
        exit 1
      fi
    EOT
  ]
}
