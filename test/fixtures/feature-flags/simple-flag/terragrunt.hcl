
feature "string_feature_flag" {
  default = "test"
}

feature "int_feature_flag" {
  default = 666
}

feature "bool_feature_flag" {
  default = false
}

inputs = {
  string_feature_flag = feature.string_feature_flag.value
  int_feature_flag = feature.int_feature_flag.value
  bool_feature_flag = feature.bool_feature_flag.value
}
