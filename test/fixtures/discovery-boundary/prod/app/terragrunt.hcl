terraform {
  source = "."
}

dependency "db" {
  config_path = "../db"
  mock_outputs = {
    db_id = "db-mocked"
  }
}

dependency "dns" {
  config_path = "../../shared/dns"
  mock_outputs = {
    dns_id = "dns-mocked"
  }
}

inputs = {
  db_id  = dependency.db.outputs.db_id
  dns_id = dependency.dns.outputs.dns_id
}
