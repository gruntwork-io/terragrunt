resource "local_file" "marker" {
  content  = "added"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "added"
}
