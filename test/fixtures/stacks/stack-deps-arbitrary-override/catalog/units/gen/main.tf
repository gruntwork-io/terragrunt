resource "local_file" "marker" {
  content  = "gen"
  filename = "${path.module}/marker.txt"
}
