generate = {
  test = {
    path      = "test.tf"
    if_exists = "overwrite"
    contents  = <<EOF
output "text" {
  value = var.text
}
EOF
  }
}
