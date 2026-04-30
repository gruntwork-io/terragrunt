removed {
  from = module.dev.module.s3.aws_s3_bucket.static_assets
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.ddb.aws_dynamodb_table.asset_metadata
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_role.lambda_role
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_policy.lambda_s3_read
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_policy.lambda_dynamodb
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_policy.lambda_basic_execution
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_s3_read
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_dynamodb
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.iam.aws_iam_role_policy_attachment.lambda_basic_execution
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.lambda.aws_lambda_function.main
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.dev.module.lambda.aws_lambda_function_url.main
  lifecycle {
    destroy = false
  }
}
