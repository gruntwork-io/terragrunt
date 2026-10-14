variable "mid_id" {
  type = string
}

output "id" {
  value = "app-${var.mid_id}"
}
