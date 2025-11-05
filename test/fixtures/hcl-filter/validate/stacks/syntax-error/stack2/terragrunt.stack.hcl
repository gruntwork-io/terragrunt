# Invalid syntax in stack file - missing value for path
unit "unit1" {
  source = "git::https://github.com/example/repo.git//modules/unit1"
  path =
}

