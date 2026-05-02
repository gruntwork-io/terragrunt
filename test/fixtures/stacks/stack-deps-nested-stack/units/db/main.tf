resource "local_file" "marker" {
  content  = "db"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-db"
}
