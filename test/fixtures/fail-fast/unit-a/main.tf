
output "data" {
  value = "unit-a"
}

resource "null_resource" "fail_if_marker_present" {
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    when    = "create"
    command = "test -f ${path.module}/fail.txt && echo 'Failing on apply due to fail.txt' && exit 1 || echo 'No fail.txt on apply, continuing...'"
  }

  provisioner "local-exec" {
    when    = "destroy"
    command = "test -f ${path.module}/fail.txt && echo 'Failing on destroy due to fail.txt' && exit 1 || echo 'No fail.txt on destroy, continuing...'"
  }
}
