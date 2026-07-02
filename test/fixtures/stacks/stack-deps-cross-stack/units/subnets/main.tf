resource "local_file" "marker" {
  content  = "subnets"
  filename = "${path.module}/marker.txt"
}

output "subnet_id" {
  value = "subnet-cross-stack"
}
