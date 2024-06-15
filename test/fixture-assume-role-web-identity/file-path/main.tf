resource "local_file" "test_file" {
  content  = "test_file"
  filename = "${path.module}/test_file.txt"
}
