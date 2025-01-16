
resource "local_file" "file" {
  content  = "chicken"
  filename = "${path.module}/test.txt"
}

output "output" {
  value = local_file.file.filename
}
