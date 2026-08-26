#!/usr/bin/env bash

case "$1" in
  "-version")
    echo "Terraform v1.0.0"
    exit 0
    ;;
  "init")
    echo "Initializing the backend..."
    exit 0
    ;;
  "output")
    cat <<'EOF'
{
  "result": {
    "sensitive": false,
    "type": "string",
    "value": "success"
  }
}
EOF
    exit 0
    ;;
  "plan")
    echo "Plan: 0 to add, 0 to change, 0 to destroy."
    exit 0
    ;;
  *)
    echo "unexpected terraform args: $*" >&2
    exit 1
    ;;
esac
