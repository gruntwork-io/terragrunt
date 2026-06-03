resource "local_file" "marker" {
  content  = "vpc-override"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "override"
}
