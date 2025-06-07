resource "null_resource" "ignore_test" {
  triggers = {
    # This will always fail
    always_fail = "true"
  }

  provisioner "local-exec" {
    command = "exit 1"
  }
}
