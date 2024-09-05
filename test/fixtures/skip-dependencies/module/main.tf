resource "local_file" "test_file" {
  content  = "test_file_content"
  filename = "${path.module}/test_file.txt"
}

