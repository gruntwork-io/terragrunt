remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket = "test-bucket"
    key    = "terraform.tfstate"
    region = "us-east-1"

    assume_role = {
      role_arn            = "arn:aws:iam::123456789342:role/test-role"
      policy_arns         = ["arn:aws:iam::123456789342:policy/test-policy", "arn:aws:iam::123456789342:policy/other-policy"]
      transitive_tag_keys = ["Project", "ProjectSlug"]
      tags = {
        Project = "test-project"
      }
    }
  }
}

terraform {
  source = "../../module"
}
