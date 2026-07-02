variable "pure_obj" {
  type = map(string)
}

variable "mixed_obj" {
  type = map(string)
}

# Looking up "pre_key" only works when the interpolated key resolved at generate time.
output "resolved" {
  value = "${join(",", keys(var.mixed_obj))}=${var.mixed_obj["pre_key"]}"
}

output "pure" {
  value = join(",", keys(var.pure_obj))
}
