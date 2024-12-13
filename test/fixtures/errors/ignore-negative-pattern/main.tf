resource "null_resource" "error_generator" {
  provisioner "local-exec" {
    command = "echo 'Error: baz' && exit 1"

    interpreter = ["/bin/sh", "-c"]
    on_failure  = fail
  }

  triggers = {
    always_run = timestamp()
  }
}
