variable "test_input1" {
  type = string
}

output "test_output1_from_parent" {
  value = var.test_input1
}

variable "test_input_map_map_string" {
  type = map(map(string))
}

output "test_output_map_map_string_from_parent" {
  value = var.test_input_map_map_string
}

variable "test_input_list_string" {
  type = list(string)
}

output "test_output_list_string" {
  value = var.test_input_list_string
}
