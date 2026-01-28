resource "null_resource" "retry_test" {
  provisioner "local-exec" {
    command = "ls success.txt"
  }
}
