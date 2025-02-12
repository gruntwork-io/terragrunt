variable "content" {
  type = string
}

variable "filename" {
  type    = string
  default = "file.txt"
}

resource "local_file" "file" {
  content  = var.content
  filename = "${path.module}/${var.filename}"
}

output "output" {
  value = local_file.file.filename
}
