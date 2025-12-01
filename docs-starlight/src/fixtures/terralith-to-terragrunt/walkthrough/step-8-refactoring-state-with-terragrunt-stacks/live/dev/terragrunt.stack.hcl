locals {
  name       = "best-cat-2025-09-24-2359-dev"
  aws_region = "us-east-1"

  units_path = find_in_parent_folders("catalog/units")
}

unit "ddb" {
  source = "${local.units_path}/ddb"
  path   = "ddb"

  values = {
    name = local.name
  }
}

unit "s3" {
  source = "${local.units_path}/s3"
  path   = "s3"

  values = {
    name = local.name

    # Optional: Force destroy S3 buckets even when they have objects in them.
    # You're generally advised not to do this with important infrastructure,
    # however this makes testing and cleanup easier for this guide.
    force_destroy = true
  }
}

unit "iam" {
  source = "${local.units_path}/iam"
  path   = "iam"

  values = {
    name = local.name

    aws_region = local.aws_region

    s3_path  = "../s3"
    ddb_path = "../ddb"
  }
}

unit "lambda" {
  source = "${local.units_path}/lambda"
  path   = "lambda"

  values = {
    name = local.name

    aws_region = local.aws_region

    s3_path  = "../s3"
    ddb_path = "../ddb"
    iam_path = "../iam"
  }
}
