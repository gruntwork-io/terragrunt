resource "local_file" "output_marker" {
  content  = "Hello from unit-w-outputs!"
  filename = "${path.module}/output.txt"
}

output "val" {
  value = "Hello!"
}
