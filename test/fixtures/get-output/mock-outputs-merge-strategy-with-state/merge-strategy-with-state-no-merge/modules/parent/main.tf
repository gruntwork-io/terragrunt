output "test_output1" {
  value = "value1"
}

output "test_output_map_map_string" {
  value = {
    map_root1 = {
      map_root1_sub1 = "map_root1_sub1_value"
    }
    not_in_state = {
      abc = "123"
    }
  }
}

output "test_output_list_string" {
  value = [
    "a",
    "b",
    "c"
  ]
}
