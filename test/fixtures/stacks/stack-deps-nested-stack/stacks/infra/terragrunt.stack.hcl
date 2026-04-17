unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

stack "deep" {
  source = "../deep"
  path   = "deep"
}
