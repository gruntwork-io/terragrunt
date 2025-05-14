
resource "local_file" "file" {
  content  = "father"
  filename = "${path.module}/test.txt"
}

output "output" {
  value = local_file.file.filename
}
