variable "region" {
  type    = string
  default = "unsupported"
}

resource "terraform_data" "guarded" {
  lifecycle {
    precondition {
      condition     = var.region != "unsupported"
      error_message = "Known safe error: region not supported."
    }
  }
}
