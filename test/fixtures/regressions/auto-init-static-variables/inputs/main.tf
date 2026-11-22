variable "src" {}

variable "other" {}

module "selected" {
  source = var.src
}
