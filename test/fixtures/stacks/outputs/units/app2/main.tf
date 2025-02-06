output "data" {
  value = "app2"
}

output "custom_value2" {
  value = "value2"
}

output "complex" {
  value = {
    name      = "name2"
    id        = 2
    timestamp = timestamp()
    delta     = 0.02
  }
}

output "list" {
  value = ["a", "b", "c"]
}

output "complex_list" {
  value = [
    {
      name      = "name2"
      id        = 2
      timestamp = timestamp()
      delta     = 0.02
    },
    {
      name      = "name3"
      id        = 2
      timestamp = timestamp()
      delta     = 0.03
    }
  ]
}