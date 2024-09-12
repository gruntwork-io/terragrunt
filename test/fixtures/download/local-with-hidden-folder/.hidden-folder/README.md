# Hidden folder

This is a "hidden folder" (one that starts with a dot) that we use to check that Terragrunt does not copy hidden 
folders--such as .git or .terraform--to the temporary folder when downloading remote Terraform configurations.
 
This file has intentionally been made read-only so that if it is copied to the temp folder, the second time you run 
Terragrunt, it will exit with an error as it tries to overwrite this read-only file.