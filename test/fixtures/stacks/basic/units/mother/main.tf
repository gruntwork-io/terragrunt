
resource "local_file" "file" {
  content  = "mother"
  filename = "${path.module}/test.txt"
}

output "output" {
  value = local_file.file.filename
}
