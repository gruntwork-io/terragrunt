terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "script_runner" {
  provisioner "local-exec" {
    command = "./script.sh 10"

    interpreter = ["/bin/sh", "-c"]
    on_failure  = fail
  }

  triggers = {
    always_run = timestamp()
  }
}
