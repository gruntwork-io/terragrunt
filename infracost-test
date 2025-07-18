provider "aws" {
  region = "us-east-1"
  skip_credentials_validation = true
  skip_requesting_account_id = true
  access_key = "mock_access_key"
  secret_key = "mock_secret_key"
}

resource "aws_instance" "my_web_app" {
  ami = "ami-005e54dee72cc1d00"

  instance_type = "m3.xlarge" # <<<<<<<<<< Try changing this to m5.xlarge to compare the costs

  tags = {
    Environment = "production"
    Service = "web-app"
  }

  root_block_device {
    volume_size = 1000 # <<<<<<<<<< Try adding volume_type="gp3" to compare costs
  }
}

resource "aws_lambda_function" "my_hello_world" {
  runtime = "nodejs12.x"
  handler = "exports.test"
  image_uri = "test"
  function_name = "test"
  role = "arn:aws:ec2:us-east-1:123123123123:instance/i-1231231231"

  memory_size = 512
  tags = {
    Environment = "Prod"
  }
}
