variable "app1_text" {
  type = string
}

output "app1_text" {
  value = var.app1_text
}

output "app2_text" {
  value = "app2 saw: ${var.app1_text}"
}
