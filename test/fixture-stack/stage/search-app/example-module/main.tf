# Create an arbitrary local resource
data "template_file" "text" {
  template = "Example text from a module"
}

output "text" {
  value = data.template_file.text.rendered
}
