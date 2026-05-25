resource "local_file" "marker" {
  content  = "idp-applied"
  filename = "${path.module}/marker.txt"
}
