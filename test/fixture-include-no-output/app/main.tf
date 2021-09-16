
resource "local_file" "main_file" {
  content     = "main_file"
  filename = "${path.module}/main_file.txt"
}

output "main_file" {
  value = local_file.main_file.filename
}

