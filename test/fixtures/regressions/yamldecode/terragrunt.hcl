inputs = {
  test1 = yamldecode("a: '003'")["a"]
  test2 = yamldecode("b: '1.00'")["b"]
  test3 = yamldecode("c: 0ba")["c"]
}
