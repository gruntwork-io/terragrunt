terraform {
  source = "."
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    vpc_id = "vpc-12345"
  }
}

dependency "db" {
  config_path = "../db"
  mock_outputs = {
    db_id = "db-vpc-12345"
  }
}

dependency "cache" {
  config_path = "../cache"
  mock_outputs = {
    cache_id = "cache-vpc-12345"
  }
}

inputs = {
  vpc_id   = dependency.vpc.outputs.vpc_id
  db_id    = dependency.db.outputs.db_id
  cache_id = dependency.cache.outputs.cache_id
}

