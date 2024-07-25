include {
  path = find_in_parent_folders()
}

locals {
  // Doing this just to confirm that credentials are available
  // while parsing a dependency.
  aws_account_id = get_aws_account_id()
}
