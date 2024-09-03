locals {
  x = 2
  file_contents     = file("./contents.txt")
  number_expression = 40+local.x
}

inputs = {
  data = local.file_contents
  answer = local.number_expression
}
