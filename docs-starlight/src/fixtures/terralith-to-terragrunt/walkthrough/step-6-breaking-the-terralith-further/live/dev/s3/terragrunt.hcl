include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//s3"
}

inputs = {
  name = "best-cat-2025-09-24-2359-dev"

  # Optional: Force destroy S3 buckets even when they have objects in them.
  # You're generally advised not to do this with important infrastructure,
  # however this makes testing and cleanup easier for this guide.
  force_destroy = true
}
