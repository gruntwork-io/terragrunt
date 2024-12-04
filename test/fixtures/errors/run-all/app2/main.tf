resource "null_resource" "script_runner" {
  provisioner "local-exec" {
    command = "./script.sh 3"

    interpreter = ["/bin/sh", "-c"]
    on_failure  = fail
  }

  triggers = {
    always_run = timestamp()
  }
}
