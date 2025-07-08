
output "data" {
  value = "unit-a"
}

resource "null_resource" "fail_execution" {
  provisioner "local-exec" {
    command = "echo 'Intentional failure' && exit 1"
  }
}
