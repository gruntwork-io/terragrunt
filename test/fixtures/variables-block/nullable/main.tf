variable "env_kept" {
  default = "default"
}

variable "tfvar_kept" {
  default = "default"
}

variable "env_dropped" {
  default = "default"
}

variable "tfvar_dropped" {
  default = "default"
}

output "values" {
  value = {
    env_kept      = var.env_kept == null ? "null" : var.env_kept
    tfvar_kept    = var.tfvar_kept == null ? "null" : var.tfvar_kept
    env_dropped   = var.env_dropped == null ? "null" : var.env_dropped
    tfvar_dropped = var.tfvar_dropped == null ? "null" : var.tfvar_dropped
  }
}
