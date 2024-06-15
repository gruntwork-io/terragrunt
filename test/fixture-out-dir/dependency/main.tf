resource "local_file" "file" {
  content  = "dependency file"
  filename = "${path.module}/dependency_file.txt"
}

output "result" {

  value = "42"
}