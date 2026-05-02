resource "local_file" "marker" {
  content  = "unit-e"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-e"
}
