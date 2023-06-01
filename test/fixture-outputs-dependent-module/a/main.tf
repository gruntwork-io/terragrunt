resource "local_file" "file" {
  content         = "module-a"
  filename        = "test.txt"
  file_permission = "0644"
}


output "value" {
  value = local_file.file.filename
}