terraform {
  backend "local" {}
}

resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "echo 'Testing disable_init functionality'"
  }
}
