module github.com/gruntwork-io/terragrunt

go 1.18

require (
	cloud.google.com/go/storage v1.16.0
	github.com/aws/aws-sdk-go v1.42.43
	github.com/creack/pty v1.1.11
	github.com/fatih/structs v1.1.0
	github.com/go-errors/errors v1.0.2-0.20180813162953-d98b870cc4e0
	github.com/google/shlex v0.0.0-20181106134648-c34317bd91bf
	github.com/gruntwork-io/terratest v0.32.6
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-getter v1.6.2
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-safetemp v1.0.0
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hcl/v2 v2.14.1

	// Many functions of terraform was converted to internal to avoid use as a library after v0.15.3. This means that we
	// can't use terraform as a library after v0.15.3, so we pull that in here.
	github.com/hashicorp/terraform v0.15.3
	github.com/hashicorp/terraform-config-inspect v0.0.0-20210318070130-9a80970d6b34
	github.com/imdario/mergo v0.3.11
	github.com/mattn/go-zglob v0.0.2-0.20190814121620-e3c945676326
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.3.3
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.2
	github.com/urfave/cli v1.22.3
	github.com/zclconf/go-cty v1.11.1
	go.mozilla.org/sops/v3 v3.7.2
	golang.org/x/crypto v0.0.0-20220517005047-85d78b3ac167
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.1.0
	google.golang.org/api v0.54.0
)

require (
	cloud.google.com/go v0.93.3 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.7 // indirect
	github.com/Microsoft/go-winio v0.4.16-0.20201130162521-d1ffc52c7331 // indirect
	github.com/containerd/continuity v0.0.0-20200710164510-efbc4488d8fe // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault // indirect
	github.com/hashicorp/vault/api v1.0.5-0.20210210214158-405eced08457 // indirect
	github.com/hashicorp/vault/sdk v0.1.14-0.20210322210658-b52b8b8c1264 // indirect
	github.com/klauspost/compress v1.13.4 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/ory/dockertest v3.3.5+incompatible // indirect
	github.com/pquerna/otp v1.2.1-0.20191009055518-468c2dd2b58d // indirect
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/terraform-linters/tflint v0.42.2
	github.com/ulikunitz/xz v0.5.10 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	google.golang.org/genproto v0.0.0-20210821163610-241b8fcbd6c8 // indirect
)

require (
	filippo.io/age v1.0.0-beta7 // indirect
	github.com/Azure/azure-sdk-for-go v52.5.0+incompatible // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.18 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.13 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.2 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmatcuk/doublestar v1.1.5 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.2+incompatible // indirect
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-github/v35 v35.3.0 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/goware/prefixer v0.0.0-20160118172347-395022866408 // indirect
	github.com/gruntwork-io/gruntwork-cli v0.7.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.3.1 // indirect
	github.com/hashicorp/go-plugin v1.4.5 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/hashicorp/terraform-registry-address v0.0.0-20220623143253-7d51757b572c // indirect
	github.com/hashicorp/terraform-svchost v0.0.0-20200729002733-f050f53b9734 // indirect
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/howeyc/gopass v0.0.0-20170109162249-bf9dde6d0d2c // indirect
	github.com/jessevdk/go-flags v1.5.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jstemmer/go-junit-report v1.0.0 // indirect
	github.com/lib/pq v1.8.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/owenrumney/go-sarif v1.1.1 // indirect
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/sourcegraph/go-lsp v0.0.0-20200429204803-219e11d77f5d // indirect
	github.com/sourcegraph/jsonrpc2 v0.1.0 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/terraform-linters/tflint-plugin-sdk v0.14.0 // indirect
	github.com/terraform-linters/tflint-ruleset-terraform v0.2.1 // indirect
	github.com/vmihailenco/msgpack/v4 v4.3.12 // indirect
	github.com/vmihailenco/tagparser v0.1.1 // indirect
	github.com/zclconf/go-cty-yaml v1.0.2 // indirect
	go.mozilla.org/gopgagent v0.0.0-20170926210634-4d7ea76ff71a // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e // indirect
	golang.org/x/net v0.1.0 // indirect
	golang.org/x/term v0.1.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.50.1 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/ini.v1 v1.44.0 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	gopkg.in/urfave/cli.v1 v1.20.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// This is necessary to workaround go modules error with terraform importing vault incorrectly.
// See https://github.com/hashicorp/vault/issues/7848 for more info
replace github.com/hashicorp/vault => github.com/hashicorp/vault v1.4.2
