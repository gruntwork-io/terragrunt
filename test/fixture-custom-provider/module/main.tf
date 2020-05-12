data "tg_welcome" "hello_world" {

}

output "test" {
  value = data.tg_welcome.hello_world.text
}