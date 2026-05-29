resource "local_file" "marker" {
  content  = "beta"
  filename = "${path.module}/marker.txt"
}
