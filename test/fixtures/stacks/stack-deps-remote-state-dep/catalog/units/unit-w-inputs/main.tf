resource "local_file" "input_marker" {
  content  = "input-marker"
  filename = "${path.module}/input.txt"
}
