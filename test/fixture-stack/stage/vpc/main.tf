# Create an arbitrary local resource
data "template_file" "text" {
  template = "[I am a vpc template. I have no dependencies.]"
}

output "text" {
  value = "${data.template_file.text.rendered}"
}
