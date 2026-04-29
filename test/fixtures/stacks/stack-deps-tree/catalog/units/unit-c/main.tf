resource "local_file" "marker" {
  content  = "unit-c"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-c"
}
