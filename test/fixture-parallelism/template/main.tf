terraform {
  backend "local" {}
}

variable "sleep_seconds" {
  type = number
}

resource "null_resource" "sleep" {
  provisioner "local-exec" {
    command = "sleep ${var.sleep_seconds}"
  }
}

resource "local_file" "timestamp" {
  content    = timestamp()
  filename   = "${path.module}/timestamp.txt"
  depends_on = [null_resource.sleep]
}

output "out" {
  value = local_file.timestamp.content
}
