package config_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestResolveUnitQueueConfigExposesAnIncludeWithAMissingDependenciesDir(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

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
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.ExcludeBlock}

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
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

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
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.DependencyBlock}

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
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

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
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.RemoteStateBlock}

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
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

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

	sts, _ := memSTS()

	v := venv.MemVenv().WithFS(fsys).WithHTTP(sts).WithEnv(map[string]string{"AWS_REGION": "us-east-1"})
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.ExcludeBlock}
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

func TestResolveUnitQueueConfigAssumesTheRoleOfAnExposedIncludeWithoutItsSessionName(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
iam_role                     = "arn:aws:iam::222222222222:role/parent"
iam_web_identity_token       = "parent-token"
iam_assume_role_session_name = "parent-session"

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

	sts, sessions := memSTS()

	v := venv.MemVenv().WithFS(fsys).WithHTTP(sts).WithEnv(map[string]string{"AWS_REGION": "us-east-1"})
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.ExcludeBlock}
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
	assert.NotContains(t, sessions(), "parent-session")
}

func TestResolveUnitQueueConfigReportsWhyAnExposedIncludeFailedToResolve(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"root.hcl": `
locals {
  skip = true
  dir  = get_env("TG_TEST_UNSET_LOCAL")
}

dependency "dep" {
  config_path  = "../dep"
  mock_outputs = { id = "mock" }
}
`,
		"dep/terragrunt.hcl": "",
		"unit/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

exclude {
  if      = include.root.locals.skip
  actions = ["all"]
}
`,
	})(t)

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestUnitParsingContext(t, path)
	run.PartialParseDecodeList = []config.PartialDecodeSectionType{config.ExcludeBlock}

	_, err := (&config.Store{}).ResolveUnitQueueConfig(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(v, run),
		path,
	)
	require.Error(t, err)

	var files []string

	for _, diags := range walkErrors(t, err).diags {
		for _, diag := range diags {
			if diag.Subject != nil {
				files = append(files, diag.Subject.Filename)
			}
		}
	}

	assert.Contains(t, files, filepath.Join(root, "root.hcl"))
}

// memSTS returns an STS endpoint that assumes any role with a web identity token, and the session names of the roles
// it assumed. The caller identity of an assumed role is the role ARN, in the account the ARN names.
func memSTS() (vhttp.Client, func() []string) {
	var (
		mu       sync.Mutex
		sessions []string
		roles    []string
	)

	sts := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		mu.Lock()
		defer mu.Unlock()

		if form.Get("Action") == "AssumeRoleWithWebIdentity" {
			sessions = append(sessions, form.Get("RoleSessionName"))
			roles = append(roles, form.Get("RoleArn"))
			accessKeyID := strconv.Itoa(len(roles) - 1)

			return vhttp.Respond(http.StatusOK, []byte(assumedRoleResponse(accessKeyID)), nil), nil
		}

		_, credential, _ := strings.Cut(req.Header.Get("Authorization"), "Credential=")
		accessKeyID, _, _ := strings.Cut(credential, "/")

		role, err := strconv.Atoi(accessKeyID)
		if err != nil || role >= len(roles) {
			return vhttp.Respond(http.StatusForbidden, nil, nil), nil
		}

		return vhttp.Respond(http.StatusOK, []byte(callerIdentityResponse(roles[role])), nil), nil
	})

	return sts, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return slices.Clone(sessions)
	}
}

// assumedRoleResponse returns an STS AssumeRoleWithWebIdentity response whose access key ID is accessKeyID.
func assumedRoleResponse(accessKeyID string) string {
	return `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>` + accessKeyID + `</AccessKeyId>
      <SecretAccessKey>fake-secret</SecretAccessKey>
      <SessionToken>fake-session</SessionToken>
      <Expiration>2100-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`
}

// callerIdentityResponse returns an STS GetCallerIdentity response whose identity is role, in the account its ARN
// names.
func callerIdentityResponse(role string) string {
	account := strings.Split(role, ":")[4]

	return `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>` + role + `</Arn>
    <UserId>AROATEST:session</UserId>
    <Account>` + account + `</Account>
  </GetCallerIdentityResult>
</GetCallerIdentityResponse>`
}
