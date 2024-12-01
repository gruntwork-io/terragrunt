resource "null_resource" "error_generator" {
  provisioner "local-exec" {
    command = "echo 'Generating example1 error' && exit 1"

    interpreter = ["/bin/sh", "-c"]
    on_failure  = fail
  }

  triggers = {
    always_run = timestamp()
  }
}
