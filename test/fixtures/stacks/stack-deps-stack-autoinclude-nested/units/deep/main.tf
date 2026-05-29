resource "local_file" "marker" {
  content  = "deep"
  filename = "${path.module}/marker.txt"
}

output "deep_id" {
  value = "deep-from-nested-autoinclude"
}
