module "main" {
  source = "../../catalog/modules/best_cat"

  name = var.name

  aws_region = var.aws_region

  lambda_zip_file = var.lambda_zip_file
  force_destroy   = var.force_destroy
}
