resource "local_file" "marker" {
  content  = "producer"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "real-producer-output"
}
