locals = {
  file_contents     = file("./contents.txt")
  number_expression = 40+2
}

inputs = {
  data = local.file_contents
  answer = local.number_expression
}
