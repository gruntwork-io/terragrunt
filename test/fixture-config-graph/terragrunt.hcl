local {
  source-prefix = "src-"
}
globals {
  region = "us-west-2"
  source-postfix = null
}
terraform {
  source = "${local.source-prefix}${global.source-postfix}"
}
