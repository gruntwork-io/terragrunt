// Used to ensure that the "terraform init" that follows a recoverable
// error will run with colors disabled.
terraform {
  extra_arguments "no-color" {
    commands = [
      "init",
    ]

    arguments = [
      "-no-color"
    ]
  }
}