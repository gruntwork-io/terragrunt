data "template_file" "foo" {
  template = "foo"
}

output "foo" {
  value = data.template_file.foo.rendered
}