# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "local"
  config = {}
}

generate "backend" {
  path      = "http.tf"
  if_exists = "overwrite"
  contents  = <<EOF
variable "url" {}

data "http" "test" {
  url = var.url
}

output "out" {
  value = data.http.test.body
}
EOF
}

inputs = {
  terraform_remote_state_s3_bucket = "__FILL_IN_BUCKET_NAME__"
}
