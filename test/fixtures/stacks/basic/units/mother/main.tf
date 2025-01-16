
resource "local_file" "file" {
  content  = "test"
  filename = "${path.module}/test.txt"
}

output "output" {
  value = local_file.file.filename
}
