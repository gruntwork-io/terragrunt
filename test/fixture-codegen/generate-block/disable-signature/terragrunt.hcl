generate "json" {
  path              = "data.json"
  if_exists         = "overwrite"
  disable_signature = true
  contents          = <<EOF
{
  "text": "Hello, World!"
}
EOF
}
