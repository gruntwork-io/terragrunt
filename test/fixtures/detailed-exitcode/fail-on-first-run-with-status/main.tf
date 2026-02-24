data "external" "script" {
  program = [
    "/bin/bash",
    "-c",
    <<EOT
      set -euo pipefail
      if [[ -f .file ]]; then
        jq -n '{foo}'
        rm .file
      else
        touch .file
        exit 1
      fi
    EOT
  ]
}

output "foo" {
  value = data.external.script.result.foo
}
