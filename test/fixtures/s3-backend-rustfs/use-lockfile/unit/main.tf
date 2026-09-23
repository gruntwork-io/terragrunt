terraform {
  backend "s3" {}
}

variable "value" {
  type    = string
  default = "initial"
}

# While this file exists, creating the resource blocks, so a test can hold the
# state lock for as long as it needs. The wait gives up after 120 seconds.
variable "gate_path" {
  type    = string
  default = ""
}

resource "terraform_data" "value" {
  input = var.value

  provisioner "local-exec" {
    command = "i=0; while [ -e \"${var.gate_path}\" ] && [ $i -lt 1200 ]; do sleep 0.1; i=$((i+1)); done"
  }
}

output "value" {
  value = terraform_data.value.output
}
