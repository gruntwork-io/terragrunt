
feature "string_feature_flag" {
  default = "test"
}

feature "int_feature_flag" {
  default = 666
}

feature "bool_feature_flag" {
  default = false
}

terraform {
  source = "."

  before_hook "conditional_command" {
    commands = ["apply", "plan", "destroy"]
    execute  = feature.bool_feature_flag.value ? ["sh", "-c", "echo running conditional bool_feature_flag"] : [ "sh", "-c", "exit", "0" ]
  }
}

inputs = {
  string_feature_flag = feature.string_feature_flag.value
  int_feature_flag = feature.int_feature_flag.value
  bool_feature_flag = feature.bool_feature_flag.value
}
