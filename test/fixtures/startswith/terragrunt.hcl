inputs = {
  startswith1 = startswith("hello world", "hello")
  startswith2 = startswith("hello world", "world")
  startswith3 = startswith("hello world", "")
  startswith4 = startswith("hello world", " ")
  startswith5 = startswith("", "")
  startswith6 = startswith("", " ")
  startswith7 = startswith(" ", "")
  startswith8 = startswith("", "hello")
  startswith9 = startswith(" ", "hello")
}
