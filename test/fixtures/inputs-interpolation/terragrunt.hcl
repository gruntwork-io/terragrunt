inputs = {
  map_with_interpolation    = jsondecode(file("stuff.json"))
  string_with_interpolation = file("stuff.json")
  any_with_interpolation    = file("stuff.json")
}
