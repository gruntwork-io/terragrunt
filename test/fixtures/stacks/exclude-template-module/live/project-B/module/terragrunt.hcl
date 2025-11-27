terraform {
  source = "."
}

# This will fail parsing if module/ isn't excluded from discovery
# because ${values} only exists in generated stack context, not during standalone parsing
inputs = {
  environment = values.env
}

