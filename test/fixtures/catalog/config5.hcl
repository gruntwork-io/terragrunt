catalog {
  urls = [
    "github.com/gruntwork-io/terraform-aws-eks",
    "github.com/gruntwork-io/terraform-aws-vpc",
  ]

  discovery {
    urls = ["github.com/acme-corp/infrastructure"]
    module_paths = ["infra"]
  }

  discovery {
    urls = ["github.com/acme-corp/platform"]
    module_paths = ["terraform"]
  }
}
