removed {
  from = module.s3.aws_s3_bucket.static_assets
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.ddb.aws_dynamodb_table.asset_metadata
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.lambda.aws_lambda_function.main
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.lambda.aws_lambda_function_url.main
  lifecycle {
    destroy = false
  }
}
