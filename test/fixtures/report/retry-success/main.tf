variable "config_dir" {
  description = "Original terragrunt config directory"
  type        = string
}

resource "null_resource" "retry_test" {
  provisioner "local-exec" {
    command = "ls ${var.config_dir}/success.txt"
  }
}
