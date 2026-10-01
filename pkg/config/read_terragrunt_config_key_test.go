package config_test

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// keyedParsingContextFields changes each exported [config.ParsingContext] field that is part of
// [config.ReadTerragruntConfigKey]. Fields of [config.ParserSettings] are named Parser.<field>.
var keyedParsingContextFields = map[string]func(*config.ParsingContext){
	"TerraformCliArgs": func(pctx *config.ParsingContext) {
		pctx.TerraformCliArgs = iacargs.New().SetCommand("plan")
	},
	"FeatureFlags": func(pctx *config.ParsingContext) {
		pctx.FeatureFlags = map[string]string{"flag": "on"}
	},
	"FilesRead": func(pctx *config.ParsingContext) { pctx.FilesRead = config.NewFilesRead() },
	"TerragruntConfigPath": func(pctx *config.ParsingContext) {
		pctx.TerragruntConfigPath = filepath.Join("other", config.DefaultTerragruntConfigPath)
	},
	"DownloadDir":               func(pctx *config.ParsingContext) { pctx.DownloadDir = "other" },
	"Source":                    func(pctx *config.ParsingContext) { pctx.Source = "other" },
	"TerraformCommand":          func(pctx *config.ParsingContext) { pctx.TerraformCommand = "plan" },
	"OriginalTerraformCommand":  func(pctx *config.ParsingContext) { pctx.OriginalTerraformCommand = "plan" },
	"TerragruntStackConfigPath": func(pctx *config.ParsingContext) { pctx.TerragruntStackConfigPath = "other" },
	"IAMRoleOptions": func(pctx *config.ParsingContext) {
		pctx.IAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/other"}
	},
	"OriginalIAMRoleOptions": func(pctx *config.ParsingContext) {
		pctx.OriginalIAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::111111111111:role/other"}
	},
	"PartialParseDecodeList": func(pctx *config.ParsingContext) {
		pctx.PartialParseDecodeList = []config.PartialDecodeSectionType{config.FeatureFlagsBlock}
	},
	"MaxFoldersToCheck":   func(pctx *config.ParsingContext) { pctx.MaxFoldersToCheck++ },
	"SkipOutput":          func(pctx *config.ParsingContext) { pctx.SkipOutput = !pctx.SkipOutput },
	"TFPathExplicitlySet": func(pctx *config.ParsingContext) { pctx.TFPathExplicitlySet = !pctx.TFPathExplicitlySet },
	"Parser.HaltOnErrorOnlyInBlocks": func(pctx *config.ParsingContext) {
		pctx.Parser.HaltOnErrorOnlyInBlocks = []string{"locals"}
	},
	"Parser.Diagnostics": func(pctx *config.ParsingContext) { pctx.Parser.Diagnostics = config.DiagnosticsSuppressed },
	"Parser.RewriteBareInclude": func(pctx *config.ParsingContext) {
		pctx.Parser.RewriteBareInclude = !pctx.Parser.RewriteBareInclude
	},
	"Parser.IgnoreDiagnostics": func(pctx *config.ParsingContext) {
		pctx.Parser.IgnoreDiagnostics = !pctx.Parser.IgnoreDiagnostics
	},
	"Parser.SkipDefaults": func(pctx *config.ParsingContext) { pctx.Parser.SkipDefaults = !pctx.Parser.SkipDefaults },
}

// keyedUnexportedParsingContextFields lists the unexported [config.ParsingContext] fields that are part of
// [config.ReadTerragruntConfigKey]. Only code inside the package sets them, so no test here can change them.
var keyedUnexportedParsingContextFields = []string{"stubWorkingDirFunc", "catalogOnly", "skipAutoIncludeMerge"}

// Reasons a [config.ParsingContext] field can stay out of [config.ReadTerragruntConfigKey].
const (
	clearedByRead      = "the read clears it before parsing the file"
	logTextOnly        = "it only changes how paths appear in logs and errors"
	sameForCommand     = "it's the same for every unit in a command, and the cache lasts one command"
	stacksAndDepsOnly  = "only stack generation and dependency runs read it"
	dependencyRunsOnly = "only dependency output fetches read it, and files with dependency blocks aren't shared"
)

// parsingContextFieldsOutsideKey gives, for each [config.ParsingContext] field left out of
// [config.ReadTerragruntConfigKey], why a change to it can't change the value a read_terragrunt_config read returns.
var parsingContextFieldsOutsideKey = map[string]string{
	"TrackInclude":                 "ParseConfig clears it before decoding the file",
	"DecodedDependencies":          clearedByRead,
	"SkipOutputsResolution":        clearedByRead,
	"Locals":                       clearedByRead,
	"Features":                     clearedByRead,
	"Values":                       "ParseConfig replaces it with the file's own values before evaluating anything",
	"WorkingDir":                   "the read sets it from the file's path, which is in the key",
	"OriginalTerragruntConfigPath": "every function that reads it stops the result being shared",
	"SourceMap": "only get_working_dir, the remote state source URL, dependencies, and stack files read it, and " +
		"none of those results are shared",
	"ReadConfigChain":                  "it only detects cycles, which fail the read, and failed reads aren't cached",
	"ParseDepth":                       "it only limits nesting, and going past the limit fails the read",
	"RootWorkingDir":                   logTextOnly,
	"LogShowAbsPaths":                  logTextOnly,
	"Experiments":                      sameForCommand,
	"StrictControls":                   sameForCommand,
	"UsePartialParseConfigCache":       "it chooses whether partial parses are cached, not what they return",
	"ScaffoldRootFileName":             "only catalog scaffolding reads it",
	"ProviderCacheOptions":             "parsing doesn't read it",
	"NoStackValidate":                  "only stack files read it, and they aren't cached",
	"NoCAS":                            stacksAndDepsOnly,
	"CASCloneDepth":                    stacksAndDepsOnly,
	"CASProbeTTL":                      stacksAndDepsOnly,
	"CASOffline":                       stacksAndDepsOnly,
	"CASRefresh":                       stacksAndDepsOnly,
	"EngineConfig":                     dependencyRunsOnly,
	"EngineOptions":                    dependencyRunsOnly,
	"Telemetry":                        dependencyRunsOnly,
	"AuthProviderCmd":                  dependencyRunsOnly,
	"TFPath":                           dependencyRunsOnly,
	"TofuImplementation":               dependencyRunsOnly,
	"ForwardTFStdout":                  dependencyRunsOnly,
	"JSONLogFormat":                    dependencyRunsOnly,
	"Debug":                            dependencyRunsOnly,
	"AutoInit":                         dependencyRunsOnly,
	"Headless":                         dependencyRunsOnly,
	"BackendBootstrap":                 dependencyRunsOnly,
	"CheckDependentUnits":              dependencyRunsOnly,
	"LogDisableErrorSummary":           dependencyRunsOnly,
	"NoDependencyFetchOutputFromState": dependencyRunsOnly,
	"dependencyOutputEnvKeys":          dependencyRunsOnly,
	"Parser.DiagnosticsHandler":        "a read with a diagnostics handler skips the cache",
}

// TestReadTerragruntConfigKeyCoversParsingContext pins that every ParsingContext field is either part of the
// read_terragrunt_config cache key or listed with the reason it doesn't need to be. A new field fails this test until
// someone decides which it is.
func TestReadTerragruntConfigKeyCoversParsingContext(t *testing.T) {
	t.Parallel()

	fields := parsingContextFieldNames()

	for _, name := range fields {
		_, keyed := keyedParsingContextFields[name]
		_, outside := parsingContextFieldsOutsideKey[name]
		keyedUnexported := slices.Contains(keyedUnexportedParsingContextFields, name)

		assert.True(
			t,
			keyed || outside || keyedUnexported,
			"ParsingContext field %s is neither in the read_terragrunt_config cache key nor listed as outside it; "+
				"add it to ReadTerragruntConfigKey and keyedParsingContextFields, or explain in "+
				"parsingContextFieldsOutsideKey why it can't change a read's result",
			name,
		)
	}

	for name := range keyedParsingContextFields {
		assert.Contains(t, fields, name, "keyedParsingContextFields lists a field ParsingContext doesn't have")
	}

	for name := range parsingContextFieldsOutsideKey {
		assert.Contains(t, fields, name, "parsingContextFieldsOutsideKey lists a field ParsingContext doesn't have")
	}

	for _, name := range keyedUnexportedParsingContextFields {
		assert.Contains(t, fields, name, "keyedUnexportedParsingContextFields lists a field ParsingContext doesn't have")
	}
}

func TestReadTerragruntConfigKeyChangesWithEachKeyedField(t *testing.T) {
	t.Parallel()

	for name, change := range keyedParsingContextFields {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New()
			cfgPath := filepath.Join(t.TempDir(), config.DefaultTerragruntConfigPath)

			_, base := newTestParsingContext(t, cfgPath)
			_, changed := newTestParsingContext(t, cfgPath)
			change(changed)

			assert.NotEqual(
				t,
				config.NewReadTerragruntConfigKey(v, base, 0),
				config.NewReadTerragruntConfigKey(v, changed, 0),
			)
		})
	}
}

// parsingContextFieldNames returns the names of the fields of [config.ParsingContext], with the fields of its Parser
// settings in place of Parser.
func parsingContextFieldNames() []string {
	var names []string

	for field := range reflect.TypeFor[config.ParsingContext]().Fields() {
		if field.Name != "Parser" {
			names = append(names, field.Name)
			continue
		}

		for parserField := range field.Type.Fields() {
			names = append(names, "Parser."+parserField.Name)
		}
	}

	return names
}
