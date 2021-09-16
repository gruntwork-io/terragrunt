resource "local_file" "module_main_file" {
  content  = "module_main_file"
  filename = "${path.module}/module_main_file.txt"
}
