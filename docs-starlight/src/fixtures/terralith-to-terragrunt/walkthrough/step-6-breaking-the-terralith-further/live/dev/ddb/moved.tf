moved {
  from = module.ddb.aws_dynamodb_table.asset_metadata
  to   = aws_dynamodb_table.asset_metadata
}
