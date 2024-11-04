include "root" {
  path   = find_in_parent_folders("common.hcl")
}

inputs = {
  string_feature_flag = feature.string_feature_flag.value
  int_feature_flag = feature.int_feature_flag.value
  bool_feature_flag = feature.bool_feature_flag.value
}
