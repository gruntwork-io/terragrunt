package config_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// funcCacheReason names what a table entry claims about the function it exercises, and with
// it what the case has to prove on top of the differential every case runs.
type funcCacheReason int

const (
	// reasonIdenticalAcrossChildren is a function whose result cannot vary with the unit a
	// shared parent is decoded for, so one decode may answer for every child.
	reasonIdenticalAcrossChildren funcCacheReason = iota
	// reasonValueVariesPerChild is a function that answers differently for each child, so a
	// reused decode would hand every later child the first child's answer.
	reasonValueVariesPerChild
	// reasonSideEffectPerChild is a function that records something on the child as it runs.
	// Its value can match between children and the classification still hold, because a
	// reused decode would skip the recording for every child after the first.
	reasonSideEffectPerChild
)

// funcCacheCallSite names which file of the fixture a case writes the call under test into. A
// parent's locals and its feature defaults are cleared by separate arms of the cacheability
// walk, and a sibling autoinclude is a third file the cache can reach, so a table whose cases
// all land in one of them leaves the others answering to nothing.
type funcCacheCallSite int

const (
	// callSiteParentLocals writes the call into the shared parent's own locals.
	callSiteParentLocals funcCacheCallSite = iota
	// callSiteParentFeature writes the call into a feature default the parent's locals then
	// read back. Defaults are decoded before the file's own locals and are cleared by the
	// walk's feature arm rather than its locals arm.
	callSiteParentFeature
	// callSiteSharedAutoInclude writes the call into a terragrunt.autoinclude.hcl beside the
	// shared parent, whose contribution reaches every unit that pulls that parent in. The
	// autoinclude is parsed with the unit's own config path and carries no include of its own,
	// so a function answering from the include answers from the unit itself there.
	callSiteSharedAutoInclude
)

// files returns the fixture files that put call at this site, keyed by a path relative to the
// fixture root.
func (s funcCacheCallSite) files(call string) map[string]string {
	files := map[string]string{"root.hcl": fmt.Sprintf(funcCacheLocalsParent, call)}

	switch s {
	case callSiteParentLocals:
		// The parent built above is already the locals site.
	case callSiteParentFeature:
		files["root.hcl"] = fmt.Sprintf(funcCacheFeatureParent, call)
	case callSiteSharedAutoInclude:
		files["root.hcl"] = funcCacheSharedParent
		files[config.DefaultAutoIncludeFile] = fmt.Sprintf(funcCacheLocalsParent, call)
	}

	return files
}

// supportingCacheableFiles is how many files a site's fixture holds, besides the one carrying
// the call under test, that the cache is meant to decode. The counters report on the whole
// parse, so an assertion about the call's own file has to leave room for these.
func (s funcCacheCallSite) supportingCacheableFiles() int64 {
	if s == callSiteSharedAutoInclude {
		return 1
	}

	return 0
}

// cacheState says whether a parse runs with the base blocks cache on or with the kill switch
// set, so the two halves of the differential read as themselves at the call site.
type cacheState int

const (
	cacheEnabled cacheState = iota
	cacheDisabled
)

// String names the state for an assertion message.
func (s cacheState) String() string {
	if s == cacheDisabled {
		return "with the cache off"
	}

	return "with the cache on"
}

// funcCacheUnit is one of the units a case parses.
type funcCacheUnit struct {
	// name labels the unit in the fixture and in assertion messages.
	name string
	// dir is the unit's directory, relative to the fixture root.
	dir string
	// parent is the path the unit's include block points at, relative to dir.
	parent string
	// command is the OpenTofu command the run is executing for this unit.
	command string
	// source is what --source resolves to for this unit, which a run rewrites per unit by
	// appending the unit's path to the root the flag names.
	source string
}

// funcCacheCase is one entry of the classification table: a fixture shared by two units in
// which one classified function is called once, and what the case has to prove about it.
type funcCacheCase struct {
	// call is the expression the fixture evaluates.
	call string
	// files holds the fixture files the call needs, keyed by a path relative to the fixture
	// root, on top of the ones the call site builds and the two units every case gets.
	files map[string]string
	// env is the environment both units parse with.
	env map[string]string
	// unitEnv adds to env for one unit, keyed by unit name and then by variable name. It is
	// how a case gives its two units the differing per-unit state a run would give them.
	unitEnv map[string]map[string]string
	// effect names the file, relative to each unit's own directory, that a
	// reasonSideEffectPerChild call has to record on that unit's read set. Every other
	// reason leaves it empty.
	effect string
	// reason is what the case claims about the function, and so what it asserts.
	reason funcCacheReason
	// callSite is where in the fixture the call is written. The zero value puts it in the
	// shared parent's locals.
	callSite funcCacheCallSite
}

// funcCacheUnits are the two units every case parses. They sit at different depths under one
// shared parent, and each carries the per-unit state a run hands its units: its own command,
// its own arguments down to its own plan file, and --source rewritten for its own path.
var funcCacheUnits = []funcCacheUnit{
	{
		name:    "a",
		dir:     "a",
		parent:  "../root.hcl",
		command: "plan",
		source:  fixtureRoot + "/modules//a",
	},
	{
		name:    "b",
		dir:     filepath.Join("nested", "b"),
		parent:  "../../root.hcl",
		command: "apply",
		source:  fixtureRoot + "/modules//nested/b",
	},
}

// funcCacheLocalsParent holds the call under test as its only local. One call per file means a
// misclassification changes exactly one value. It is the shared parent at
// [callSiteParentLocals] and the sibling autoinclude at [callSiteSharedAutoInclude].
const funcCacheLocalsParent = `
locals {
  value = %s
}

inputs = {
  value = local.value
}
`

// funcCacheFeatureParent is the shared parent a [callSiteParentFeature] case builds. The call
// under test is the default of a feature flag, which the locals read back so the value still
// reaches the unit's inputs.
const funcCacheFeatureParent = `
feature "value" {
  default = %s
}

locals {
  value = feature.value.value
}

inputs = {
  value = local.value
}
`

// funcCacheSharedParent is the shared parent a [callSiteSharedAutoInclude] case builds. The
// call under test lives in the autoinclude beside it, so this file holds nothing that could
// vary per unit and is the one file of that fixture the cache may decode.
const funcCacheSharedParent = `
locals {
  parent = "shared"
}

inputs = {
  parent = local.parent
}
`

func TestClassifiedFuncNamesBehaveAsClassified(t *testing.T) {
	t.Parallel()

	cases := funcCacheCases()

	for _, name := range config.ClassifiedFuncNames() {
		tc, exercised := cases[name]
		if !assert.True(
			t,
			exercised,
			"%s is classified in funcNameChildIndependence but no case in funcCacheCases exercises it: add one, so the classification is proven rather than asserted",
			name,
		) {
			continue
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			runFuncCacheCase(t, name, tc)
		})
	}

	for name := range cases {
		_, classified := config.FuncNameIsChildIndependent(name)
		assert.True(t, classified, "%s has a case but is not a classified function name", name)
	}
}

// runFuncCacheCase parses the case's two units with the cache on and again with it off, and
// holds the result to what the entry claims.
func runFuncCacheCase(t *testing.T, name string, tc funcCacheCase) {
	t.Helper()

	independent, classified := config.FuncNameIsChildIndependent(name)
	require.True(t, classified, "%s is not classified", name)
	require.Equal(
		t,
		independent,
		tc.reason == reasonIdenticalAcrossChildren,
		"the case for %s claims a reason its classification contradicts",
		name,
	)

	enabled, enabledCtxs, enabledCache := parseFuncCaseUnits(t, tc, cacheEnabled)
	disabled, disabledCtxs, disabledCache := parseFuncCaseUnits(t, tc, cacheDisabled)

	require.Zero(
		t,
		disabledCache.Hits()+disabledCache.Misses(),
		"the kill switch did not turn the cache off",
	)

	for _, unit := range funcCacheUnits {
		assert.Equal(
			t,
			disabled[unit.name],
			enabled[unit.name],
			"unit %s parsed differently with the cache on",
			unit.name,
		)
		assert.Equal(
			t,
			unit.name,
			enabled[unit.name].Inputs["unit"],
			"unit %s was handed another unit's inputs",
			unit.name,
		)
		// Emptiness rather than absence: a call the parse never reached leaves the key
		// missing, and a call that ran but answered with nothing leaves the comparisons below
		// true of two values neither of which says anything about the function.
		require.NotEmpty(
			t,
			disabled[unit.name].Inputs["value"],
			"%s produced nothing for unit %s, so this case no longer shows what it answers",
			name,
			unit.name,
		)
	}

	// The values compared below come from the parse with the cache off, which is what each
	// unit computes for itself. Comparing what the cache handed back would only report on
	// the cache.
	first, second := disabled[funcCacheUnits[0].name], disabled[funcCacheUnits[1].name]

	switch tc.reason {
	case reasonIdenticalAcrossChildren:
		require.Equal(
			t,
			tc.callSite.supportingCacheableFiles()+1,
			enabledCache.Misses(),
			"the file calling %s was decoded more than once",
			name,
		)
		require.Positive(
			t,
			enabledCache.Hits(),
			"the file calling %s was never served from the cache",
			name,
		)
		assert.Equal(
			t,
			first.Inputs["value"],
			second.Inputs["value"],
			"%s is classified child-independent but answered its two units differently",
			name,
		)
	case reasonValueVariesPerChild:
		requireCallUncached(t, name, tc.callSite, enabledCache)
		assert.NotEqual(
			t,
			first.Inputs["value"],
			second.Inputs["value"],
			"%s answered both units the same, so this fixture no longer shows why it cannot be cached",
			name,
		)
	case reasonSideEffectPerChild:
		requireCallUncached(t, name, tc.callSite, enabledCache)
		require.NotEmpty(t, tc.effect, "the case for %s names no file it should record", name)
	}

	// The effect is checked whatever the entry claims. Reclassifying one of these functions
	// as child-independent makes both units agree on a value, so the differential above stays
	// quiet, and the only thing left to notice is the unit whose read set the reused decode
	// never touched.
	if tc.effect != "" {
		assertEffectRecorded(t, name, tc.effect, enabledCtxs, cacheEnabled)
		assertEffectRecorded(t, name, tc.effect, disabledCtxs, cacheDisabled)
	}
}

// assertEffectRecorded holds every unit of one parse to having recorded effect, named
// relative to that unit's own directory, on its own read set.
func assertEffectRecorded(
	t *testing.T,
	name string,
	effect string,
	contexts map[string]*config.ParsingContext,
	state cacheState,
) {
	t.Helper()

	for _, unit := range funcCacheUnits {
		recorded := filepath.Join(fixtureRoot, unit.dir, effect)

		assert.Contains(
			t,
			contexts[unit.name].FilesRead.Paths(),
			recorded,
			"%s did not record %s for unit %s %s",
			name,
			recorded,
			unit.name,
			state,
		)
	}
}

// requireCallUncached holds the file calling a child-dependent function to never reaching the
// cache. A hit needs a miss on the same key ahead of it, so holding the misses down to the
// call site's supporting files accounts for the hits too.
func requireCallUncached(
	t *testing.T,
	name string,
	site funcCacheCallSite,
	cache *config.BaseBlocksCache,
) {
	t.Helper()

	require.Equal(
		t,
		site.supportingCacheableFiles(),
		cache.Misses(),
		"the cache decoded the file calling %s",
		name,
	)
}

// parseFuncCaseUnits writes the case's fixture into memory and parses both units against one
// cache, returning what each unit decoded, the parsing context it decoded with, and the cache
// they shared. Every unit gets a parsing context and a venv of its own, the way a run gives
// each unit its own while sharing the caches on the context, and the per-unit state below is
// what the runner fills in for a unit before it parses it.
func parseFuncCaseUnits(
	t *testing.T,
	tc funcCacheCase,
	state cacheState,
) (map[string]*config.TerragruntConfig, map[string]*config.ParsingContext, *config.BaseBlocksCache) {
	t.Helper()

	files := tc.callSite.files(tc.call)
	maps.Copy(files, tc.files)

	for _, unit := range funcCacheUnits {
		files[filepath.Join(unit.dir, "terragrunt.hcl")] = childIncluding(unit.parent, unit.name)
	}

	fsys := venvtest.NewFS(t, fixtureRoot, files)
	base := venvtest.New().
		WithFS(fsys).
		WithHandler(fixtureExec(fsys)).
		WithHTTP(vhttp.NewMemClient(fixtureAWS())).
		WithSops(vsops.NewMemDecrypter(fixtureSops()))

	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	configs := make(map[string]*config.TerragruntConfig, len(funcCacheUnits))
	contexts := make(map[string]*config.ParsingContext, len(funcCacheUnits))

	for _, unit := range funcCacheUnits {
		path := filepath.Join(fixtureRoot, unit.dir, config.DefaultTerragruntConfigPath)

		_, pctx := newTestParsingContext(t, base.WithEnv(funcCaseEnv(tc, unit.name, state)), path)
		pctx.OriginalTerragruntConfigPath = path
		pctx.TerraformCommand = unit.command
		pctx.TerraformCliArgs = iacargs.New(
			unit.command,
			"-out="+filepath.Join(fixtureRoot, unit.dir, "tfplan"),
		)
		pctx.Source = unit.source
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.DeepMerge))

		cfg, err := config.ParseConfigFile(ctx, pctx, l, path, nil)
		require.NoError(t, err, "unit %s", unit.name)

		configs[unit.name] = cfg
		contexts[unit.name] = pctx
	}

	return configs, contexts, config.ContextBaseBlocksCache(ctx)
}

// funcCaseEnv returns the environment the unit called unitName parses with.
func funcCaseEnv(tc funcCacheCase, unitName string, state cacheState) map[string]string {
	env := map[string]string{}
	maps.Copy(env, tc.env)
	maps.Copy(env, tc.unitEnv[unitName])

	if state == cacheDisabled {
		env[config.EnvDisableBaseBlocksCache] = "true"
	}

	return env
}

// errUnexpectedInvocation is what the fixture services answer with when a case reaches for
// something no case planned for, so an unexercised path fails instead of passing silently.
var errUnexpectedInvocation = errors.New("the fixture does not answer this")

// maxFixtureGitScan bounds the walk up the fixture tree looking for a repository marker.
const maxFixtureGitScan = 64

// awsIdentity is what STS and IAM report for one set of credentials.
type awsIdentity struct {
	// account is the account id GetCallerIdentity reports.
	account string
	// alias is the account alias ListAccountAliases reports.
	alias string
	// arn is the caller ARN GetCallerIdentity reports.
	arn string
	// userID is the user id GetCallerIdentity reports.
	userID string
}

// fixtureAWSIdentities maps an access key id to the identity AWS reports for it.
//
// A run assumes each unit's iam_role before parsing that unit and writes the resulting
// session onto that unit's own venv, and the AWS config builder takes those credentials over
// re-assuming the role. Two units that selected different roles therefore reach AWS as
// different principals, which is the divergence these entries stand in for.
var fixtureAWSIdentities = map[string]awsIdentity{
	"AKIAFIXTUREUNITA": {
		account: "111111111111",
		alias:   "fixture-unit-a",
		arn:     "arn:aws:sts::111111111111:assumed-role/unit-a/parse",
		userID:  "AROAFIXTUREUNITA:parse",
	},
	"AKIAFIXTUREUNITB": {
		account: "222222222222",
		alias:   "fixture-unit-b",
		arn:     "arn:aws:sts::222222222222:assumed-role/unit-b/parse",
		userID:  "AROAFIXTUREUNITB:parse",
	},
}

// awsUnitEnv gives each unit the AWS session a run would have left on its venv after
// assuming that unit's own IAM role.
var awsUnitEnv = map[string]map[string]string{
	"a": {
		"AWS_ACCESS_KEY_ID":     "AKIAFIXTUREUNITA",
		"AWS_SECRET_ACCESS_KEY": "fixture-secret-a",
		"AWS_SESSION_TOKEN":     "fixture-token-a",
	},
	"b": {
		"AWS_ACCESS_KEY_ID":     "AKIAFIXTUREUNITB",
		"AWS_SECRET_ACCESS_KEY": "fixture-secret-b",
		"AWS_SESSION_TOKEN":     "fixture-token-b",
	},
}

// funcCacheCases returns one entry per classified Terragrunt HCL function, keyed by the
// function's name. [TestClassifiedFuncNamesBehaveAsClassified] walks the classification
// itself and fails on any name missing from here, so a function added to Terragrunt has to
// be both classified and exercised before the suite goes green again.
func funcCacheCases() map[string]funcCacheCase {
	return map[string]funcCacheCase{
		// Read only their arguments, the process environment, or a list baked into
		// Terragrunt, none of which the unit being parsed can change.
		config.FuncNameGetEnv: {
			call:   `get_env("TG_FIXTURE_SHARED_VALUE", "fallback")`,
			env:    map[string]string{"TG_FIXTURE_SHARED_VALUE": "shared"},
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetPlatform: {
			call:   `get_platform()`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetTerraformCommandsThatNeedVars: {
			call:   `join(",", get_terraform_commands_that_need_vars())`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetTerraformCommandsThatNeedLocking: {
			call:   `join(",", get_terraform_commands_that_need_locking())`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetTerraformCommandsThatNeedInput: {
			call:   `join(",", get_terraform_commands_that_need_input())`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetTerraformCommandsThatNeedParallelism: {
			call:   `join(",", get_terraform_commands_that_need_parallelism())`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameGetDefaultRetryableErrors: {
			call:   `join("|", get_default_retryable_errors())`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameConstraintCheck: {
			call:   `constraint_check("1.2.3", ">= 1.0.0")`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameDeepMerge: {
			call:   `deep_merge({ left = "one" }, { right = "two" })`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameStartsWith: {
			call:   `startswith("terragrunt", "terra")`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameEndsWith: {
			call:   `endswith("terragrunt", "grunt")`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameStrContains: {
			call:   `strcontains("terragrunt", "agr")`,
			reason: reasonIdenticalAcrossChildren,
		},
		config.FuncNameTimeCmp: {
			call:   `timecmp("2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z")`,
			reason: reasonIdenticalAcrossChildren,
		},

		// Answer from the command the unit being parsed is running, which the runner gives
		// each unit its own of, down to its own plan file.
		config.FuncNameGetTerraformCommand: {
			call: `get_terraform_command()`,
			// In a feature default rather than in locals, so the walk's feature arm is held to
			// refusing what its locals arm refuses. The locals arm sees this function through
			// the cliargs corpus fixture, which is expected to cache nothing.
			callSite: callSiteParentFeature,
			reason:   reasonValueVariesPerChild,
		},
		config.FuncNameGetTerraformCLIArgs: {
			call:   `join(" ", get_terraform_cli_args())`,
			reason: reasonValueVariesPerChild,
		},

		// Answer from the unit's own path. The two units sit at different depths, so a
		// parent that walks up from, or relativizes against, the unit answers differently
		// for each.
		config.FuncNameFindInParentFolders: {
			call: `find_in_parent_folders("marker.hcl")`,
			// A marker at each depth, so the unit deeper in the tree stops at the nearer one.
			files: map[string]string{
				"marker.hcl":        "",
				"nested/marker.hcl": "",
			},
			reason: reasonValueVariesPerChild,
		},
		config.FuncNamePathRelativeToInclude: {
			call:   `path_relative_to_include()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNamePathRelativeFromInclude: {
			call:   `path_relative_from_include()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetTerragruntDir: {
			call:   `get_terragrunt_dir()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetOriginalTerragruntDir: {
			call:   `get_original_terragrunt_dir()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetTerragruntSourceCLIFlag: {
			call:   `get_terragrunt_source_cli_flag()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetWorkingDir: {
			call:   `get_working_dir()`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetRepoRoot: {
			call: `get_repo_root()`,
			// A repository of its own around each unit. One root cannot answer for a unit
			// that sits in another repository, which is what makes the two answers differ.
			files: map[string]string{
				"a/.git":      "",
				"nested/.git": "",
			},
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetPathFromRepoRoot: {
			call:   `get_path_from_repo_root()`,
			files:  map[string]string{".git": ""},
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameGetPathToRepoRoot: {
			call:   `get_path_to_repo_root()`,
			files:  map[string]string{".git": ""},
			reason: reasonValueVariesPerChild,
		},

		// Resolve a relative path, or a command's working directory, against the unit's own
		// directory, so each unit reaches a file or a directory of its own.
		config.FuncNameRunCmd: {
			call:   `run_cmd("pwd")`,
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameReadTerragruntConfig: {
			// The read target is JSON, which the cache refuses outright for want of an
			// hclsyntax body to classify. The parent is therefore still the only file in
			// this fixture the counters can be reporting on.
			call: `read_terragrunt_config("target.hcl.json").locals.value`,
			files: map[string]string{
				"a/target.hcl.json":        `{"locals": {"value": "read by a"}}`,
				"nested/b/target.hcl.json": `{"locals": {"value": "read by b"}}`,
			},
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameReadTfvarsFile: {
			call: `read_tfvars_file("vars.tfvars")`,
			files: map[string]string{
				"a/vars.tfvars":        `value = "read by a"`,
				"nested/b/vars.tfvars": `value = "read by b"`,
			},
			reason: reasonValueVariesPerChild,
		},
		config.FuncNameSopsDecryptFile: {
			call:   `sops_decrypt_file("secrets.yaml")`,
			reason: reasonValueVariesPerChild,
		},

		// Record the file they read on the unit's read set. Both units are handed the same
		// value and the effect is still each unit's own.
		config.FuncNameMarkAsRead: {
			call: `mark_as_read("marked.txt")`,
			files: map[string]string{
				"a/marked.txt":        "",
				"nested/b/marked.txt": "",
			},
			effect: "marked.txt",
			reason: reasonSideEffectPerChild,
		},
		config.FuncNameMarkGlobAsRead: {
			call: `join(",", mark_glob_as_read("*.txt"))`,
			files: map[string]string{
				".git":                "",
				"a/marked.txt":        "",
				"nested/b/marked.txt": "",
			},
			effect: "marked.txt",
			reason: reasonSideEffectPerChild,
		},

		// Reach AWS as the principal the unit's own credentials name.
		config.FuncNameGetAWSAccountAlias: {
			call:    `get_aws_account_alias()`,
			unitEnv: awsUnitEnv,
			reason:  reasonValueVariesPerChild,
		},
		config.FuncNameGetAWSAccountID: {
			call:    `get_aws_account_id()`,
			unitEnv: awsUnitEnv,
			reason:  reasonValueVariesPerChild,
		},
		config.FuncNameGetAWSCallerIdentityArn: {
			call:    `get_aws_caller_identity_arn()`,
			unitEnv: awsUnitEnv,
			reason:  reasonValueVariesPerChild,
		},
		config.FuncNameGetAWSCallerIdentityUserID: {
			call:    `get_aws_caller_identity_user_id()`,
			unitEnv: awsUnitEnv,
			reason:  reasonValueVariesPerChild,
		},

		// Answers from the include the unit declares, and falls back to the unit's own
		// directory when there is none to answer from. A call in the shared parent's locals
		// cannot show that: both units include the same parent, so both land on the parent's
		// directory. An autoinclude beside that parent can, because Terragrunt parses it with
		// the unit's config path and no include of its own.
		config.FuncNameGetParentTerragruntDir: {
			call:     `get_parent_terragrunt_dir()`,
			callSite: callSiteSharedAutoInclude,
			reason:   reasonValueVariesPerChild,
		},
	}
}

// fixtureExec answers the commands the fixtures shell out to. `git rev-parse --show-toplevel`
// reports the nearest directory at or above the invocation holding a repository marker, and
// `pwd` echoes the invocation's own directory, so a call made from two units is visibly two
// calls. Anything else fails, so a command no case planned for cannot pass for an answer.
func fixtureExec(fsys vfs.FS) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "pwd" {
			return vexec.Result{Stdout: []byte(inv.Dir + "\n")}
		}

		if inv.Name != "git" || !slices.Equal(inv.Args, []string{"rev-parse", "--show-toplevel"}) {
			return vexec.Result{
				Err: fmt.Errorf("%w: %s %v", errUnexpectedInvocation, inv.Name, inv.Args),
			}
		}

		root, found := nearestFixtureGitRoot(fsys, inv.Dir)
		if !found {
			return vexec.Result{ExitCode: 128, Stderr: []byte("fatal: not a git repository\n")}
		}

		return vexec.Result{Stdout: []byte(root + "\n")}
	}
}

// nearestFixtureGitRoot returns the closest directory at or above dir holding a `.git` entry.
func nearestFixtureGitRoot(fsys vfs.FS, dir string) (string, bool) {
	current := dir

	for range maxFixtureGitScan {
		if _, err := fsys.Stat(filepath.Join(current, ".git")); err == nil {
			return current, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}

		current = parent
	}

	return "", false
}

// fixtureSops answers a decrypt with cleartext naming the file it decrypted, so the path each
// unit resolved its argument to is visible in the value that unit ends up with.
func fixtureSops() vsops.Handler {
	return func(_ map[string]string, path, _ string) ([]byte, error) {
		return []byte("decrypted " + path), nil
	}
}

// fixtureAWS answers the STS and IAM calls the get_aws_* functions make, as the principal
// whose access key signed the request. It reaches no network: the AWS SDK sends through the
// venv's HTTP client, and this is that client.
func fixtureAWS() vhttp.Handler {
	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		key := signingAccessKeyID(req.Header.Get("Authorization"))

		identity, known := fixtureAWSIdentities[key]
		if !known {
			return nil, fmt.Errorf("%w: a request signed with %q", errUnexpectedInvocation, key)
		}

		switch form.Get("Action") {
		case "GetCallerIdentity":
			return xmlResponse(fmt.Sprintf(
				callerIdentityXML,
				identity.arn,
				identity.userID,
				identity.account,
			)), nil
		case "ListAccountAliases":
			return xmlResponse(fmt.Sprintf(accountAliasesXML, identity.alias)), nil
		}

		return nil, fmt.Errorf("%w: the %q action", errUnexpectedInvocation, form.Get("Action"))
	}
}

// signingAccessKeyID returns the access key id a SigV4 Authorization header was signed with.
func signingAccessKeyID(header string) string {
	_, credential, found := strings.Cut(header, "Credential=")
	if !found {
		return ""
	}

	key, _, _ := strings.Cut(credential, "/")

	return key
}

// xmlResponse wraps a body the way an AWS query protocol endpoint would.
func xmlResponse(body string) *http.Response {
	return vhttp.Respond(
		http.StatusOK,
		[]byte(body),
		http.Header{"Content-Type": []string{"text/xml"}},
	)
}

const callerIdentityXML = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>%s</Arn>
    <UserId>%s</UserId>
    <Account>%s</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>fixture</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`

const accountAliasesXML = `<ListAccountAliasesResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListAccountAliasesResult>
    <IsTruncated>false</IsTruncated>
    <AccountAliases>
      <member>%s</member>
    </AccountAliases>
  </ListAccountAliasesResult>
  <ResponseMetadata>
    <RequestId>fixture</RequestId>
  </ResponseMetadata>
</ListAccountAliasesResponse>`
