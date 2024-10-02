
resource "local_file" "test" {
  content  = "app1"
  filename = "${path.module}/test.txt"
}
