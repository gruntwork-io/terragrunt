inputs = {
  endswith1 = endswith("hello world", "world")
  endswith2 = endswith("hello world", "hello")
  endswith3 = endswith("hello world", "")
  endswith4 = endswith("hello world", " ")
  endswith5 = endswith("", "")
  endswith6 = endswith("", " ")
  endswith7 = endswith(" ", "")
  endswith8 = endswith("", "hello")
  endswith9 = endswith(" ", "hello")
}
