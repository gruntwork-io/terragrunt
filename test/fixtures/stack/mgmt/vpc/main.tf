terraform {
  backend "s3" {}
}

# Create an arbitrary local resource
data "template_file" "text" {
  template = "[I am a mgmt vpc template. I have no dependencies.]"
}

output "text" {
  value = data.template_file.text.rendered
}
