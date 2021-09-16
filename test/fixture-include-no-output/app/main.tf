resource "local_file" "main_file" {
  content  = "main_file"
  filename = "${path.module}/test_file.tfstate"
}

output "main_file" {
  value = local_file.main_file.filename
}

