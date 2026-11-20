variable "module" {
  type = string
}

output "module" {
  value = var.module
}

output "run_dir" {
  value = abspath(path.module)
}
