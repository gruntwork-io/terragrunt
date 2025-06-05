resource "null_resource" "retry_test" {
  triggers = {
    # This will fail on first attempt but succeed on retry
    # because the file will exist on retry
    file_exists = fileexists("${path.module}/success.txt")
  }

  provisioner "local-exec" {
    command = "touch ${path.module}/success.txt"
  }
}
