unit "normal_app" {
  source = "../units/normal-app"
  path   = "normal-app"
}

unit "excluded_app" {
  source = "../units/excluded-app"
  path   = "excluded-app"
}

unit "excluded_all" {
  source = "../units/excluded-all"
  path   = "excluded-all"
}

unit "not_excluded_all_except_output" {
  source = "../units/not-excluded-all-except-output"
  path   = "not-excluded-all-except-output"
}
