resource "aws_dynamodb_table" "asset_metadata" {
  name         = "${var.name}-asset-metadata"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "image_id"

  attribute {
    name = "image_id"
    type = "S"
  }

  tags = {
    Name = "${var.name}-asset-metadata"
  }
}
