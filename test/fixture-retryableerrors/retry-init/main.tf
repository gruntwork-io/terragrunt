resource "null_resource" "tf_init_message" {
  provisioner "local-exec" {
    command = "if [[ -f terraform.tfstate.backup ]]; then exit 0; else echo 'Error: Error loading modules: module foo: not found, may need to run 'terraform init'' && exit 1; fi"
    interpreter = ["/bin/bash", "-c"]
  }
}