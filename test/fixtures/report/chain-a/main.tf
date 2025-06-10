resource "null_resource" "chain_a" {
  triggers = {
    always_fail = "true"
  }

  provisioner "local-exec" {
    command = "exit 1"
  }
}
