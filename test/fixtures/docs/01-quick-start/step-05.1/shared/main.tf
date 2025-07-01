variable "content" {}
variable "output_dir" {}

resource "local_file" "file" {
  content  = var.content
  filename = "${var.output_dir}/hi.txt"
} 