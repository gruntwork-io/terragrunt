variable "test_input1" {
  type = string
}

output "test_output1_from_parent" {
  value = var.test_input1
}

variable "test_input2" {
  type = string
}

output "test_output2_from_parent" {
  value = var.test_input2
}