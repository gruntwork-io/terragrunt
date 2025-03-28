# We can't explicitly specify `tofu` or `terraform` because a CircleCI job contains either `terraform` or `tofu` binary, but not both in the same job.
terraform_binary = "./tf.sh"
