resource "local_file" "file" {
  content  = "test file"
  filename = "${path.module}/file.txt"
}
