resource "local_file" "file" {
  content  = "Hello, World!"
  filename = "${path.module}/hi.txt"
}
