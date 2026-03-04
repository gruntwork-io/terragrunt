resource "aws_s3_bucket" "static_assets" {
  bucket = "${var.name}-static-assets"

  force_destroy = var.force_destroy
}
