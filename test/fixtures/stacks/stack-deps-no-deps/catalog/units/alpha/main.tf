resource "local_file" "marker" {
  content  = "alpha"
  filename = "${path.module}/marker.txt"
}
