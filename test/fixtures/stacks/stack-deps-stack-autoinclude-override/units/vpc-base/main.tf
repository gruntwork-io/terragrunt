resource "local_file" "marker" {
  content  = "vpc-base"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "base"
}
