module github.com/gruntwork-io/terragrunt

go 1.18

require (
	cloud.google.com/go/storage v1.27.0
	github.com/aws/aws-sdk-go v1.44.122
	github.com/creack/pty v1.1.11
	github.com/fatih/structs v1.1.0
	github.com/go-errors/errors v1.0.2-0.20180813162953-d98b870cc4e0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/gruntwork-io/terratest v0.32.6
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-getter v1.7.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-safetemp v1.0.0
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hcl/v2 v2.15.0

	// Many functions of terraform was converted to internal to avoid use as a library after v0.15.3. This means that we
	// can't use terraform as a library after v0.15.3, so we pull that in here.
	github.com/hashicorp/terraform v0.15.3
	github.com/hashicorp/terraform-config-inspect v0.0.0-20210318070130-9a80970d6b34
	github.com/imdario/mergo v0.3.11
	github.com/mattn/go-zglob v0.0.2-0.20190814121620-e3c945676326
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.4.3
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.2
	github.com/urfave/cli v1.22.3
	github.com/zclconf/go-cty v1.12.1
	go.mozilla.org/sops/v3 v3.7.3
	golang.org/x/crypto v0.5.0
	golang.org/x/oauth2 v0.1.0
	golang.org/x/sync v0.0.0-20220929204114-8fcdb60fdcc0
	golang.org/x/sys v0.5.0
	google.golang.org/api v0.100.0
)

require (
	cloud.google.com/go v0.104.0 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.11 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault // indirect
	github.com/hashicorp/vault/api v1.5.0 // indirect
	github.com/hashicorp/vault/sdk v0.4.1 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/pquerna/otp v1.2.1-0.20191009055518-468c2dd2b58d // indirect
	github.com/terraform-linters/tflint v0.44.1
	github.com/ulikunitz/xz v0.5.10 // indirect
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65 // indirect
	google.golang.org/genproto v0.0.0-20221025140454-527a21cfbd71 // indirect
)

require github.com/gruntwork-io/gruntwork-cli v0.7.0

require (
	cloud.google.com/go/compute v1.10.0 // indirect
	cloud.google.com/go/iam v0.5.0 // indirect
	filippo.io/age v1.0.0 // indirect
	github.com/Azure/azure-sdk-for-go v63.3.0+incompatible // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.26 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.18 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.5 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Masterminds/semver/v3 v3.2.0 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20220407094043-a94812496cf5 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/bgentry/go-netrc v0.0.0-20140422174119-9fd32a8b3d3d // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmatcuk/doublestar v1.1.5 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.3.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-github/v35 v35.3.0 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.0 // indirect
	github.com/googleapis/gax-go/v2 v2.6.0 // indirect
	github.com/goware/prefixer v0.0.0-20160118172347-395022866408 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.4.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-plugin v1.4.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/mlock v0.1.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.3 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/hashicorp/terraform-registry-address v0.1.0 // indirect
	github.com/hashicorp/terraform-svchost v0.0.0-20200729002733-f050f53b9734 // indirect
	github.com/hashicorp/yamux v0.0.0-20211028200310-0bc27b27de87 // indirect
	github.com/howeyc/gopass v0.0.0-20210920133722-c8aef6fb66ef // indirect
	github.com/jessevdk/go-flags v1.5.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jstemmer/go-junit-report v1.0.0 // indirect
	github.com/lib/pq v1.10.5 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/owenrumney/go-sarif v1.1.1 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/sourcegraph/go-lsp v0.0.0-20200429204803-219e11d77f5d // indirect
	github.com/sourcegraph/jsonrpc2 v0.1.0 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/terraform-linters/tflint-plugin-sdk v0.15.0 // indirect
	github.com/terraform-linters/tflint-ruleset-terraform v0.2.2 // indirect
	github.com/vmihailenco/msgpack/v4 v4.3.12 // indirect
	github.com/vmihailenco/tagparser v0.1.1 // indirect
	github.com/zclconf/go-cty-yaml v1.0.3 // indirect
	go.mozilla.org/gopgagent v0.0.0-20170926210634-4d7ea76ff71a // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/term v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.51.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/ini.v1 v1.66.4 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/urfave/cli.v1 v1.20.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// This is necessary to workaround go modules error with terraform importing vault incorrectly.
// See https://github.com/hashicorp/vault/issues/7848 for more info
replace github.com/hashicorp/vault => github.com/hashicorp/vault v1.4.2
