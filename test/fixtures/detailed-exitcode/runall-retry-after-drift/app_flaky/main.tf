data "external" "flaky" {
  program = [
    "/bin/bash",
    "-c",
    <<EOT
      set -euo pipefail
      if [[ ! -f .retry_marker ]]; then
        echo "transient fail" 1>&2
        touch .retry_marker
        exit 1
      fi
      echo '{}'
    EOT
  ]
}

