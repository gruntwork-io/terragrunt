resource "local_file" "file" {
  content         = "module-b"
  filename        = "module-b.txt"
  file_permission = "0644"
}

output "value" {
  value = local_file.file.filename
}
