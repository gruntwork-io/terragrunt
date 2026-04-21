resource "local_file" "marker" {
  content  = "unit-w-outputs"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-unit-w-outputs"
}
