output "data" {
  value = "app1"
}

output "custom_value1" {
  value = "value1"
}

output "complex" {
  value = {
    name      = "name1"
    id        = 2
    timestamp = timestamp()
    delta     = 0.02
  }
}

output "list" {
  value = ["1", "2", "3"]
}

output "complex_list" {
  value = [
    {
      name      = "name1"
      id        = 10
      timestamp = timestamp()
      delta     = 0.02
    },
    {
      name      = "name10"
      id        = 20
      timestamp = timestamp()
      delta     = 0.03
    }
  ]
}