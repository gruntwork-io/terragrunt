variable "x" {
  type    = list(string)
  default = ["fallback"]
}

output "x" {
  value = join(",", var.x)
}
