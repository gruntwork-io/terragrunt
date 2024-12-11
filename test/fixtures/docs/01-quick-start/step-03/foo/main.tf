variable "content" {}

module "shared" {
  source = "../shared"

  content = var.content
}
