module github.com/gruntwork-io/terragrunt

go 1.16

require (
	cloud.google.com/go v0.93.3 // indirect
	cloud.google.com/go/storage v1.16.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.7 // indirect
	github.com/Microsoft/go-winio v0.4.16-0.20201130162521-d1ffc52c7331 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/aws/aws-sdk-go v1.41.7
	github.com/containerd/continuity v0.0.0-20200710164510-efbc4488d8fe // indirect
	github.com/creack/pty v1.1.11
	github.com/fatih/structs v1.1.0
	github.com/go-errors/errors v1.0.2-0.20180813162953-d98b870cc4e0
	github.com/go-test/deep v1.0.7 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/shlex v0.0.0-20181106134648-c34317bd91bf
	github.com/gruntwork-io/terratest v0.32.6
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-getter v1.5.11
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-retryablehttp v0.6.7 // indirect
	github.com/hashicorp/go-safetemp v1.0.0
	github.com/hashicorp/go-version v1.3.0
	github.com/hashicorp/hcl v1.0.1-vault // indirect
	github.com/hashicorp/hcl/v2 v2.10.0

	// Many functions of terraform was converted to internal to avoid use as a library after v0.15.3. This means that we
	// can't use terraform as a library after v0.15.3, so we pull that in here.
	github.com/hashicorp/terraform v0.15.3
	github.com/hashicorp/terraform-config-inspect v0.0.0-20210318070130-9a80970d6b34
	github.com/hashicorp/vault/api v1.0.5-0.20210210214158-405eced08457 // indirect
	github.com/hashicorp/vault/sdk v0.1.14-0.20210322210658-b52b8b8c1264 // indirect
	github.com/imdario/mergo v0.3.11
	github.com/klauspost/compress v1.13.4 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/mattn/go-zglob v0.0.2-0.20190814121620-e3c945676326
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.3.3
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/opencontainers/runc v1.0.0-rc9 // indirect
	github.com/ory/dockertest v3.3.5+incompatible // indirect
	github.com/pquerna/otp v1.2.1-0.20191009055518-468c2dd2b58d // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/stretchr/testify v1.6.1
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/urfave/cli v1.22.3
	github.com/zclconf/go-cty v1.8.3
	go.mozilla.org/sops/v3 v3.7.2
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210823070655-63515b42dcdf
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	google.golang.org/api v0.54.0
	google.golang.org/genproto v0.0.0-20210821163610-241b8fcbd6c8 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)

// This is necessary to workaround go modules error with terraform importing vault incorrectly.
// See https://github.com/hashicorp/vault/issues/7848 for more info
replace github.com/hashicorp/vault => github.com/hashicorp/vault v1.4.2
