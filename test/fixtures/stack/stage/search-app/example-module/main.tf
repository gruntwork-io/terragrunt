# Create an arbitrary local resource
resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "echo Hello, World!"
  }
}
