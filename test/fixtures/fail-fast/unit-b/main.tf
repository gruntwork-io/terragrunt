
output "data" {
  value = "unit-b"
}

resource "null_resource" "fail_if_marker_present" {
  provisioner "local-exec" {
    command = "test -f ${path.module}/fail.txt && echo 'Failing due to fail.txt' && exit 1 || echo 'No fail.txt, continuing...'"
  }
}
