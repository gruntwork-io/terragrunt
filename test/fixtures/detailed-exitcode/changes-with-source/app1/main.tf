resource "local_file" "example" {
  content  = "Test"
  filename = "${path.module}/example.txt"
}
