package config_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestResolveUnitQueueConfigExposesAnIncludeWithAMissingDependenciesDir(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
locals {
  skip = true
}

dependencies {
  paths = ["${get_terragrunt_dir()}/../not-generated"]
}
`,
		"unit/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

exclude {
  if      = include.root.locals.skip
  actions = ["plan"]
}
`,
	})(t)

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestParsingContext(t, path)
	run.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.ExcludeBlock}

	cfg, err := (&config.Store{}).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(v, run),
		path,
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.Exclude)
	assert.True(t, cfg.Exclude.If)
}

func TestResolveUnitQueueConfigDecodesNoUnrequestedRunPartOfAnExposedInclude(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
dependencies {
  paths = ["${get_terragrunt_dir()}/../dep"]
}

locals {
  region = "us-east-1"
}

inputs = {
  token = file("not-generated.txt")
}
`,
		"dep/terragrunt.hcl": "",
		"unit/terragrunt.hcl": `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  region = include.root.locals.region
}
`,
	})(t)

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestParsingContext(t, path)
	run.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.DependencyBlock}

	cfg, err := (&config.Store{}).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(v, run),
		path,
	)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Locals["region"])
}

func TestResolveUnitQueueConfigExposesTheRequestedRemoteState(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
dependencies {
  paths = ["${get_terragrunt_dir()}/../dep"]
}

remote_state {
  backend = "s3"

  config = {
    bucket = "state"
  }
}
`,
		"dep/terragrunt.hcl": "",
		"unit/terragrunt.hcl": `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  backend = include.root.remote_state.backend
}
`,
	})(t)

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestParsingContext(t, path)
	run.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.RemoteStateBlock}

	cfg, err := (&config.Store{}).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(v, run),
		path,
	)
	require.NoError(t, err)
	assert.Equal(t, "s3", cfg.Locals["backend"])
}

func TestResolveUnitQueueConfigEvaluatesAnExposedIncludeWithTheIAMRoleItSets(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
iam_role               = "arn:aws:iam::222222222222:role/parent"
iam_web_identity_token = "parent-token"

locals {
  account = get_aws_account_id()
}
`,
		"unit/terragrunt.hcl": `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  account = include.root.locals.account
}
`,
	})(t)

	accounts := map[string]string{
		"arn:aws:iam::111111111111:role/run":    "111111111111",
		"arn:aws:iam::222222222222:role/parent": "222222222222",
	}

	sts := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		if form.Get("Action") == "AssumeRoleWithWebIdentity" {
			return vhttp.Respond(http.StatusOK, []byte(assumedRoleResponse(accounts[form.Get("RoleArn")])), nil), nil
		}

		_, credential, _ := strings.Cut(req.Header.Get("Authorization"), "Credential=")
		account, _, _ := strings.Cut(credential, "/")

		return vhttp.Respond(http.StatusOK, []byte(callerIdentityResponse(account)), nil), nil
	})

	v := venv.MemVenv().WithFS(fsys).WithHTTP(sts).WithEnv(map[string]string{"AWS_REGION": "us-east-1"})
	ctx, run := newTestParsingContext(t, path)
	run.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.ExcludeBlock}
	run.IAMRoleOptions = iam.RoleOptions{
		RoleARN:          "arn:aws:iam::111111111111:role/run",
		WebIdentityToken: "run-token",
	}

	cfg, err := (&config.Store{}).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(v, run),
		path,
	)
	require.NoError(t, err)
	assert.Equal(t, "222222222222", cfg.Locals["account"])
}

// assumedRoleResponse returns an STS AssumeRoleWithWebIdentity response whose access key ID is account.
func assumedRoleResponse(account string) string {
	return `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>` + account + `</AccessKeyId>
      <SecretAccessKey>fake-secret</SecretAccessKey>
      <SessionToken>fake-session</SessionToken>
      <Expiration>2100-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`
}

// callerIdentityResponse returns an STS GetCallerIdentity response for account.
func callerIdentityResponse(account string) string {
	return `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::` + account + `:assumed-role/test/session</Arn>
    <UserId>AROATEST:session</UserId>
    <Account>` + account + `</Account>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`
}
