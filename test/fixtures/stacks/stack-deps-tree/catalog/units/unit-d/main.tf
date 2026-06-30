resource "local_file" "marker" {
  content  = "unit-d"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-d"
}
