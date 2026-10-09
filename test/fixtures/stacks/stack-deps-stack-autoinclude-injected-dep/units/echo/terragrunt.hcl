terraform {
  source = "."
}

inputs = {
  value = try(values.value, "unwired")
}
