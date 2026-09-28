stack "networking" {
  source = "../stacks/networking"
  path   = "networking"
}

unit "app" {
  source = "../units/app"
  path   = "app"
}

unit "strict" {
  source = "../units/strict"
  path   = "strict"
}

unit "malformed" {
  source = "../units/malformed"
  path   = "malformed"
}
