# required_input is the only variable without a default, so it is the only
# input the scaffold test expects in the generated configuration.
variable "required_input" {
  type        = string
  description = "An input the caller has to supply"
}

variable "optional_input" {
  type    = string
  default = "a default the caller can leave alone"
}
