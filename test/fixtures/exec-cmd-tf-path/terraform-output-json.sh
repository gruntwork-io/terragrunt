#!/usr/bin/env bash

# Handle -version
if [ "$1" = "-version" ]; then
  echo "Terraform v1.0.0"
  exit 0
fi

# Output variable
cat << 'EOF'
{
"baz": {
  "sensitive": false,
  "type": "string",
  "value": "terraform"
}
}
EOF