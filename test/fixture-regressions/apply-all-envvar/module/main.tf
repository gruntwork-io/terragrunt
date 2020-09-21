variable "seed" {}
output "text" {
  value = "Hello ${var.seed}"
}
