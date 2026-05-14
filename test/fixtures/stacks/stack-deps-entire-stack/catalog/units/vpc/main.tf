resource "local_file" "marker" {
  content  = "vpc"
  filename = "${path.module}/marker.txt"
}

output "vpc_id" {
  value = "vpc-entire-stack"
}
