terraform {
  source = "../../../../module/message"
}

inputs = {
  msg = try(values.msg, "the correct echo message from unit!")
}
