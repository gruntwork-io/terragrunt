module "s3" {
  source = "../s3"

  name = var.name

  force_destroy = var.force_destroy
}

module "ddb" {
  source = "../ddb"

  name = var.name
}

module "iam" {
  source = "../iam"

  name = var.name

  aws_region = var.aws_region

  s3_bucket_arn      = module.s3.arn
  dynamodb_table_arn = module.ddb.arn
}

module "lambda" {
  source = "../lambda"

  name = var.name

  aws_region = var.aws_region

  s3_bucket_name      = module.s3.name
  dynamodb_table_name = module.ddb.name
  lambda_zip_file     = var.lambda_zip_file
  lambda_role_arn     = module.iam.arn
}
