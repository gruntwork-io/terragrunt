include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//s3"
}

inputs = {
  name = values.name

  # Optional: Force destroy S3 buckets even when they have objects in them.
  force_destroy = try(values.force_destroy, false)
}
