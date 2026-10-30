terraform {
  backend "s3" {
    bucket  = "does-not-exist-mock-test"
    key     = "dep.tfstate"
    region  = "us-east-1"
    profile = "does-not-exist"
  }
}

output "x" {
  value = "real"
}
