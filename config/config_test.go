package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestTerragruntConfigStringIsEmpty(t *testing.T) {
	t.Parallel()

	testTerragruntConfig := TerragruntConfig{}

	actualResult := testTerragruntConfig.String()

	expectedOutput := "TerragruntConfig{Terraform = <nil>, RemoteState = <nil>, Dependencies = <nil>, PreventDestroy = <nil>}"

	assert.Equal(t, expectedOutput, actualResult)
}

// Note: Incoherent IAM naming accross the code.
func TestTerragruntConfigGetIAMRoleOptionsIsEmpty(t *testing.T) {
	t.Parallel()

	testTerragruntConfig := TerragruntConfig{}

	actualResult := testTerragruntConfig.GetIAMRoleOptions()

	expectedResult := options.IAMRoleOptions{}

	assert.Equal(t, expectedResult, actualResult)
}

func TestTerragruntConfigGetIAMRoleOptionsCopiesArnAndSession(t *testing.T) {
	t.Parallel()

	dummyIamRole := "my-test-role"
	dummySessionName := "my-test-session"

	testTerragruntConfig := TerragruntConfig{
		IamRole:                  dummyIamRole,
		IamAssumeRoleSessionName: dummySessionName,
	}

	actualResult := testTerragruntConfig.GetIAMRoleOptions()

	expectedResult := options.IAMRoleOptions{
		RoleARN:               dummyIamRole,
		AssumeRoleSessionName: dummySessionName,
	}

	assert.Equal(t, expectedResult, actualResult)
}

func TestTerragruntConfigGetIAMRoleOptionsRoleDurationNotEmpty(t *testing.T) {
	t.Parallel()

	dummyRoleDuration := new(int64)

	testTerragruntConfig := TerragruntConfig{
		IamAssumeRoleDuration: dummyRoleDuration,
	}

	actualResult := testTerragruntConfig.GetIAMRoleOptions()

	expectedResult := options.IAMRoleOptions{
		AssumeRoleDuration: *dummyRoleDuration,
	}

	assert.Equal(t, expectedResult, actualResult)
}

func TestRemoteStateConfigFileIsEmpty(t *testing.T) {
	t.Parallel()

	testRemoteStateConfigFile := remoteStateConfigFile{}

	actualResult := testRemoteStateConfigFile.String()

	expectedOutput := "remoteStateConfigFile{Backend = , Config = {{<nil>} <nil>}}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestRemoteStateConfigFileToConfigDefault(t *testing.T) {
	t.Parallel()

	testBackend := "my-backend"

	dummyCtyMap := make(map[string]cty.Value)
	dummyCtyMap["Backend"] = cty.StringVal(testBackend)
	stubCtyValue := cty.MapVal(dummyCtyMap)
	parsedStubCtyValue, _ := parseCtyValueToMap(stubCtyValue)

	testRemoteStateConfigFile := remoteStateConfigFile{
		Backend: testBackend,
		Config:  stubCtyValue,
	}

	actualResult, _ := testRemoteStateConfigFile.toConfig()

	expectedOutput := &remote.RemoteState{
		Backend: testBackend,
		Config:  parsedStubCtyValue,
	}
	expectedOutput.FillDefaults()

	assert.Equal(t, expectedOutput, actualResult)
}

func TestRemoteStateConfigFileToConfigUsesGenerate(t *testing.T) {
	t.Parallel()

	testBackend := "my-backend"
	testGenerate := "dummy-value-for-generate"

	stubGenerate := &remoteStateConfigGenerate{}

	dummyCtyMap := make(map[string]cty.Value)
	dummyCtyMap["Backend"] = cty.StringVal(testBackend)
	dummyCtyMap["Generate"] = cty.StringVal(testGenerate)

	stubCtyValue := cty.MapVal(dummyCtyMap)
	parsedStubCtyValue, _ := parseCtyValueToMap(stubCtyValue)

	testRemoteStateConfigFile := remoteStateConfigFile{
		Backend:  testBackend,
		Config:   stubCtyValue,
		Generate: stubGenerate,
	}

	actualResult, _ := testRemoteStateConfigFile.toConfig()

	expectedOutput := &remote.RemoteState{
		Backend:  testBackend,
		Config:   parsedStubCtyValue,
		Generate: &remote.RemoteStateGenerate{},
	}
	expectedOutput.FillDefaults()

	assert.Equal(t, expectedOutput, actualResult)
}

func TestRemoteStateConfigFileToConfigUsesDisableInit(t *testing.T) {
	t.Parallel()

	testBackend := "my-backend"
	testDisableInit := true

	dummyCtyMap := make(map[string]cty.Value)
	dummyCtyMap["Backend"] = cty.StringVal(testBackend)
	stubCtyValue := cty.MapVal(dummyCtyMap)
	parsedStubCtyValue, _ := parseCtyValueToMap(stubCtyValue)

	testRemoteStateConfigFile := remoteStateConfigFile{
		Backend:     testBackend,
		Config:      stubCtyValue,
		DisableInit: &testDisableInit,
	}

	actualResult, _ := testRemoteStateConfigFile.toConfig()

	expectedOutput := &remote.RemoteState{
		Backend:     testBackend,
		Config:      parsedStubCtyValue,
		DisableInit: testDisableInit,
	}
	expectedOutput.FillDefaults()

	assert.Equal(t, expectedOutput, actualResult)
}

func TestRemoteStateConfigFileToConfigUsesDisableDependencyOptimization(t *testing.T) {
	t.Parallel()

	testBackend := "my-backend"
	testDisableDependencyOptimization := true

	dummyCtyMap := make(map[string]cty.Value)
	dummyCtyMap["Backend"] = cty.StringVal(testBackend)
	stubCtyValue := cty.MapVal(dummyCtyMap)
	parsedStubCtyValue, _ := parseCtyValueToMap(stubCtyValue)

	testRemoteStateConfigFile := remoteStateConfigFile{
		Backend:                       testBackend,
		Config:                        stubCtyValue,
		DisableDependencyOptimization: &testDisableDependencyOptimization,
	}

	actualResult, _ := testRemoteStateConfigFile.toConfig()

	expectedOutput := &remote.RemoteState{
		Backend:                       testBackend,
		Config:                        parsedStubCtyValue,
		DisableDependencyOptimization: testDisableDependencyOptimization,
	}
	expectedOutput.FillDefaults()

	assert.Equal(t, expectedOutput, actualResult)
}

func TestIncludeConfigStringIsEmpty(t *testing.T) {
	t.Parallel()

	testIncludeConfig := IncludeConfig{}

	actualResult := testIncludeConfig.String()

	expectedOutput := "IncludeConfig{Path = , Expose = <nil>, MergeStrategy = <nil>}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestGetExposeIsFalseIfExposeIsNil(t *testing.T) {
	t.Parallel()

	testIncludeConfig := IncludeConfig{
		Expose: nil,
	}

	actualResult := testIncludeConfig.GetExpose()

	expectedOutput := false

	assert.Equal(t, expectedOutput, actualResult)
}

func TestGetExpose(t *testing.T) {
	t.Parallel()

	testExpose := true
	testIncludeConfig := IncludeConfig{
		Expose: &testExpose,
	}

	actualResult := testIncludeConfig.GetExpose()

	expectedOutput := testExpose

	assert.Equal(t, expectedOutput, actualResult)
}

func TestGetMergeStrategyNilDefaultsToShallow(t *testing.T) {
	t.Parallel()

	testIncludeConfig := IncludeConfig{
		MergeStrategy: nil,
	}

	actualResult, _ := testIncludeConfig.GetMergeStrategy()

	expectedOutput := ShallowMerge

	assert.Equal(t, expectedOutput, actualResult)
}

func TestGetMergeStrategyDefaultsToNoMerge(t *testing.T) {
	t.Parallel()

	testMergeStrategy := ""
	testIncludeConfig := IncludeConfig{
		MergeStrategy: &testMergeStrategy,
	}

	actualResult, _ := testIncludeConfig.GetMergeStrategy()

	expectedOutput := NoMerge

	assert.Equal(t, expectedOutput, actualResult)
}

func TestModuleDependenciesStringIsEmpty(t *testing.T) {
	t.Parallel()

	testModuleDependencies := ModuleDependencies{}

	actualResult := testModuleDependencies.String()

	expectedOutput := "ModuleDependencies{Paths = []}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestHookStringIsEmpty(t *testing.T) {
	t.Parallel()

	testHook := Hook{}

	actualResult := testHook.String()

	expectedOutput := "Hook{Name = , Commands = 0}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestErrorHookStringIsEmpty(t *testing.T) {
	t.Parallel()

	testErrorHook := ErrorHook{}

	actualResult := testErrorHook.String()

	expectedOutput := "Hook{Name = , Commands = 0}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformConfigStringIsEmpty(t *testing.T) {
	t.Parallel()

	testTerraformConfig := TerraformConfig{}

	actualResult := testTerraformConfig.String()

	expectedOutput := "TerraformConfig{Source = <nil>}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformConfigGetBeforeHooks(t *testing.T) {
	t.Parallel()

	testBeforeHooks := make([]Hook, 0)
	testTerraformConfig := TerraformConfig{
		BeforeHooks: testBeforeHooks,
	}

	actualResult := testTerraformConfig.GetBeforeHooks()

	expectedOutput := testBeforeHooks

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformConfigGetAfterHooks(t *testing.T) {
	t.Parallel()

	testAfterHooks := make([]Hook, 0)
	testTerraformConfig := TerraformConfig{
		AfterHooks: testAfterHooks,
	}

	actualResult := testTerraformConfig.GetAfterHooks()

	expectedOutput := testAfterHooks

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformConfigGetErrorHooks(t *testing.T) {
	t.Parallel()

	testErrorHooks := make([]ErrorHook, 0)
	testTerraformConfig := TerraformConfig{
		ErrorHooks: testErrorHooks,
	}

	actualResult := testTerraformConfig.GetErrorHooks()

	expectedOutput := testErrorHooks

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformConfigValidateHooksNoHooksNoError(t *testing.T) {
	t.Parallel()

	testTerraformConfig := TerraformConfig{}

	actualResult := testTerraformConfig.ValidateHooks()

	assert.Nil(t, actualResult)
}

func TestTerraformConfigValidateHooksBeforeHooksError(t *testing.T) {
	t.Parallel()

	stubHook := Hook{}

	testBeforeHooks := make([]Hook, 1)
	testBeforeHooks = append(testBeforeHooks, stubHook)
	testTerraformConfig := TerraformConfig{
		BeforeHooks: testBeforeHooks,
	}

	actualResult := testTerraformConfig.ValidateHooks()

	assert.Error(t, actualResult)
}

func TestTerraformConfigValidateHooksAftereHooksError(t *testing.T) {
	t.Parallel()

	stubHook := Hook{}

	testAfterHooks := make([]Hook, 1)
	testAfterHooks = append(testAfterHooks, stubHook)
	testTerraformConfig := TerraformConfig{
		AfterHooks: testAfterHooks,
	}

	actualResult := testTerraformConfig.ValidateHooks()

	assert.Error(t, actualResult)
}

func TestTerraformConfigValidateHooksErrorHooksError(t *testing.T) {
	t.Parallel()

	stubHook := ErrorHook{}

	testErrorHooks := make([]ErrorHook, 1)
	testErrorHooks = append(testErrorHooks, stubHook)
	testTerraformConfig := TerraformConfig{
		ErrorHooks: testErrorHooks,
	}

	actualResult := testTerraformConfig.ValidateHooks()

	assert.Error(t, actualResult)
}

func TestTerraformExtraArgumentsStringIsEmpty(t *testing.T) {
	t.Parallel()

	testTerraformExtraArguments := TerraformExtraArguments{}

	actualResult := testTerraformExtraArguments.String()

	expectedOutput := "TerraformArguments{Name = , Arguments = <nil>, Commands = [], EnvVars = <nil>}"

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformExtraArgumentsGetVarFilesIsEmpty(t *testing.T) {
	t.Parallel()

	mockLogger := logrus.StandardLogger()

	testTerraformExtraArguments := TerraformExtraArguments{}
	testLogger := logrus.NewEntry(mockLogger)

	actualResult := testTerraformExtraArguments.GetVarFiles(testLogger)

	expectedOutput := make([]string, 0)

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformExtraArgumentsGetVarFilesRequiredVarFiles(t *testing.T) {
	t.Parallel()

	dummyFile := "someFile.hcl"
	dummyRequiredVarFiles := []string{dummyFile, dummyFile}
	mockLogger := logrus.StandardLogger()

	testTerraformExtraArguments := TerraformExtraArguments{
		RequiredVarFiles: &dummyRequiredVarFiles,
	}
	testLogger := logrus.NewEntry(mockLogger)

	actualResult := testTerraformExtraArguments.GetVarFiles(testLogger)

	expectedOutput := []string{dummyFile}

	assert.Equal(t, expectedOutput, actualResult)
}

func TestTerraformExtraArgumentsGetVarFilesOptionalVarFiles(t *testing.T) {
	t.Parallel()

	dummyFile := "someFile.hcl"
	dummyOptionalVarFiles := []string{dummyFile, dummyFile}
	mockLogger := logrus.StandardLogger()

	testTerraformExtraArguments := TerraformExtraArguments{
		OptionalVarFiles: &dummyOptionalVarFiles,
	}
	testLogger := logrus.NewEntry(mockLogger)

	actualResult := testTerraformExtraArguments.GetVarFiles(testLogger)

	// Note: False positive
	// Because util.FileExists currently not working in test
	// Monkey patching is currently not possible
	expectedOutput := []string{}

	assert.Equal(t, expectedOutput, actualResult)
}

func TestGetTerraformSourceUrlReturnsConfigSource(t *testing.T) {
	t.Parallel()

	dummySource := "some-path"
	testTerragruntOptions := options.TerragruntOptions{
		Source: dummySource,
	}
	testTerragruntConfig := TerragruntConfig{}

	actualResult, _ := GetTerraformSourceUrl(&testTerragruntOptions, &testTerragruntConfig)

	assert.Equal(t, dummySource, actualResult)
}

func TestGetTerraformSourceUrlReturnsTerraformSource(t *testing.T) {
	t.Parallel()

	dummySource := "some-path"
	dummyConfig := TerraformConfig{
		Source: &dummySource,
	}

	testTerragruntOptions := options.TerragruntOptions{}
	testTerragruntConfig := TerragruntConfig{
		Terraform: &dummyConfig,
	}

	actualResult, _ := GetTerraformSourceUrl(&testTerragruntOptions, &testTerragruntConfig)

	assert.Equal(t, dummySource, actualResult)
}

func TestGetTerraformSourceUrlNoSourceIsEmptyl(t *testing.T) {
	t.Parallel()

	testTerragruntOptions := options.TerragruntOptions{}
	testTerragruntConfig := TerragruntConfig{}

	actualResult, _ := GetTerraformSourceUrl(&testTerragruntOptions, &testTerragruntConfig)

	assert.Equal(t, "", actualResult)
}

func TestAdjustSourceWithMapSourcemapEmptyReturnsSource(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{}
	testSource := "test-source"
	testModulePath := ""

	actualResult, _ := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testSource, actualResult)
}

func TestAdjustSourceWithMapModuleUrlAndSubdirEmptyError(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{"key": "value"}
	testSource := "//"
	testModulePath := ""

	_, err := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Error(t, err)
}

func TestAdjustSourceWithMapModuleUrlEmptyReturnsSource(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{"key": "value"}
	testSource := "//not-empty"
	testModulePath := ""

	actualResult, _ := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testSource, actualResult)
}

func TestAdjustSourceWithMapModuleUrlParseErrorReturnsSource(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{"key": "value"}
	testSource := ":not-empty-but-invalid://"
	testModulePath := ""

	actualResult, err := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testSource, actualResult)
	assert.Error(t, err)
}

func TestAdjustSourceWithMapNoKeyInMapReturnsSource(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{"key": "value"}
	testSource := "not-empty//"
	testModulePath := ""

	actualResult, err := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testSource, actualResult)
	assert.NoError(t, err)
}

func TestAdjustSourceWithMapReplaceSubdirWithKeyInMapWithError(t *testing.T) {
	t.Parallel()

	testSourceMap := map[string]string{"not-empty": "my-path"}
	testSource := "not-empty//"
	testModulePath := ""

	actualResult, err := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testModulePath, actualResult)
	assert.Error(t, err)
}

func TestAdjustSourceWithMapReplaceSubdirWithKeyInMap(t *testing.T) {
	t.Parallel()

	dummyMapPathPart := "my-path"
	dummySourcePathPart := "not-empty"

	testSourceMap := map[string]string{"http://my-url.com/not-empty": dummyMapPathPart}
	testSource := "http://my-url.com/" + dummySourcePathPart + "//"
	testModulePath := dummyMapPathPart + "//" + dummySourcePathPart

	actualResult, err := adjustSourceWithMap(testSourceMap, testSource, testModulePath)

	assert.Equal(t, testModulePath, actualResult)
	assert.NoError(t, err)
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()

	testWorkingDir := "my/test/dir"

	actualResult := DefaultConfigPath(testWorkingDir)

	expectedResult := testWorkingDir + "/" + DefaultTerragruntConfigPath

	assert.Equal(t, expectedResult, actualResult)
}

func TestDefaultJsonConfigPath(t *testing.T) {
	t.Parallel()

	testWorkingDir := "my/test/dir"

	actualResult := DefaultJsonConfigPath(testWorkingDir)

	expectedResult := testWorkingDir + "/" + DefaultTerragruntJsonConfigPath

	assert.Equal(t, expectedResult, actualResult)
}

func TestGetDefaultConfigPathDefault(t *testing.T) {
	t.Parallel()

	testWorkingDir := "my/test/dir"

	actualResult := GetDefaultConfigPath(testWorkingDir)

	expectedResult := testWorkingDir + "/" + DefaultTerragruntConfigPath

	assert.Equal(t, expectedResult, actualResult)
}

func TestFindConfigFilesInPath(t *testing.T) {
	t.Parallel()

	testRootPath := ""
	terragruntOptions := options.TerragruntOptions{}

	actualResult, err := FindConfigFilesInPath(testRootPath, &terragruntOptions)

	expectedResult := []string{}

	// Note: Will require monkey patching
	// Or better mocking for all cases to be tested
	assert.Equal(t, expectedResult, actualResult)
	assert.Error(t, err)
}

func TestContainsTerragruntModuleNotIsDirReturnsFalse(t *testing.T) {
	t.Parallel()

	dummyIsDir := false

	testPath := ""
	testInfo := MockOsFileInfo{
		isDir: dummyIsDir,
	}
	testTerragruntOptions := options.TerragruntOptions{}

	actualResult, err := containsTerragruntModule(testPath, testInfo, &testTerragruntOptions)

	expectedResult := dummyIsDir

	assert.Equal(t, expectedResult, actualResult)
	assert.NoError(t, err)
}

func TestContainsTerragruntModuleContainsPathReturnsFalse(t *testing.T) {
	t.Parallel()

	testPath := options.TerragruntCacheDir
	testInfo := MockOsFileInfo{
		isDir: true,
	}
	testTerragruntOptions := options.TerragruntOptions{}

	actualResult, err := containsTerragruntModule(testPath, testInfo, &testTerragruntOptions)

	expectedResult := false

	assert.Equal(t, expectedResult, actualResult)
	assert.NoError(t, err)
}

func TestContainsTerragruntModuleDataDirHasPathPrefixReturnsFalse(t *testing.T) {
	t.Parallel()

	dummyEnv := map[string]string{"TF_DATA_DIR": "/.terraform"}

	testPath := "/.terraform/modules"
	testInfo := MockOsFileInfo{
		isDir: true,
	}
	testTerragruntOptions := options.TerragruntOptions{
		Env: dummyEnv,
	}

	actualResult, err := containsTerragruntModule(testPath, testInfo, &testTerragruntOptions)

	expectedResult := false

	assert.Equal(t, expectedResult, actualResult)
	assert.NoError(t, err)
}

func TestContainsTerragruntModuleDataDirNotAbsoluteReturnsFalse(t *testing.T) {
	t.Parallel()

	dummyEnv := map[string]string{"TF_DATA_DIR": "modules"}

	testPath := "modules/my-module"
	testInfo := MockOsFileInfo{
		isDir: true,
	}
	testTerragruntOptions := options.TerragruntOptions{
		Env: dummyEnv,
	}

	actualResult, err := containsTerragruntModule(testPath, testInfo, &testTerragruntOptions)

	expectedResult := false

	assert.Equal(t, expectedResult, actualResult)
	assert.NoError(t, err)
}

func TestContainsTerragruntModuleCanonicalInDownloadPathReturnsFalse(t *testing.T) {
	t.Parallel()

	dummyPathPart := "my-module"

	testPath := "/modules/" + dummyPathPart
	testInfo := MockOsFileInfo{
		isDir: true,
	}
	testTerragruntOptions := options.TerragruntOptions{
		DownloadDir: "/modules/" + dummyPathPart,
	}

	actualResult, err := containsTerragruntModule(testPath, testInfo, &testTerragruntOptions)

	expectedResult := false

	assert.Equal(t, expectedResult, actualResult)
	// Note: Actually this should be Error but there is a bug in the code
	assert.NoError(t, err)
}

// TODO: Test throws a SegFault because of derefenrencing a nil pointer
//
// func TestReadTerragruntConfigFailsReadingFile(t *testing.T) {
// 	t.Parallel()

// 	dummyFilename := ""
// 	testTerraformOptions := options.TerragruntOptions{
// 		TerragruntConfigPath: dummyFilename,
// 	}

// 	actualResult, err := ReadTerragruntConfig(&testTerraformOptions)

// 	assert.Nil(t, actualResult)
// 	assert.Error(t, err)
// }

func TestParseConfigFileFailsReadingFile(t *testing.T) {
	t.Parallel()

	testFilename := ""
	testTerraformOptions := options.TerragruntOptions{}
	testInclude := IncludeConfig{}
	testDependencyOutputs := cty.StringVal("fill-in-something")

	actualResult, err := ParseConfigFile(testFilename, &testTerraformOptions, &testInclude, &testDependencyOutputs)

	assert.Nil(t, actualResult)
	assert.Error(t, err)
}

func TestParseTerragruntConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state {
  backend = "s3"
  config  = {}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntConfigRemoteStateAttrMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state = {
  backend = "s3"
  config  = {}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntJsonConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"remote_state": {
		"backend": "s3",
		"config": {}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntHclConfigRemoteStateMissingBackend(t *testing.T) {
	t.Parallel()

	config := `
remote_state {}
`

	_, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing required argument; The argument \"backend\" is required")
}

func TestParseTerragruntHclConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state {
	backend = "s3"
	config = {
  		encrypt = true
  		bucket = "my-bucket"
  		key = "terraform.tfstate"
  		region = "us-east-1"
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}
}

func TestParseTerragruntJsonConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"remote_state":{
		"backend":"s3",
		"config":{
			"encrypt": true,
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region":"us-east-1"
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}
}

func TestParseTerragruntHclConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	config := `
retry_max_attempts = 10
retry_sleep_interval_sec = 60
retryable_errors = [
    "My own little error",
    "Another one of my errors"
]
`
	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	assert.Equal(t, 10, *terragruntConfig.RetryMaxAttempts)
	assert.Equal(t, 60, *terragruntConfig.RetrySleepIntervalSec)

	if assert.NotNil(t, terragruntConfig.RetryableErrors) {
		assert.Equal(t, []string{"My own little error", "Another one of my errors"}, terragruntConfig.RetryableErrors)
	}
}

func TestParseTerragruntJsonConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	config := `
{
	"retry_max_attempts": 10,
	"retry_sleep_interval_sec": 60,
	"retryable_errors": [
        "My own little error"
	]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	assert.Equal(t, *terragruntConfig.RetryMaxAttempts, 10)
	assert.Equal(t, *terragruntConfig.RetrySleepIntervalSec, 60)

	if assert.NotNil(t, terragruntConfig.RetryableErrors) {
		assert.Equal(t, []string{"My own little error"}, terragruntConfig.RetryableErrors)
	}
}

func TestParseIamRole(t *testing.T) {
	t.Parallel()

	config := `iam_role = "terragrunt-iam-role"`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, "terragrunt-iam-role", terragruntConfig.IamRole)
}

func TestParseIamAssumeRoleDuration(t *testing.T) {
	t.Parallel()

	config := `iam_assume_role_duration = 36000`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, int64(36000), *terragruntConfig.IamAssumeRoleDuration)
}

func TestParseIamAssumeRoleSessionName(t *testing.T) {
	t.Parallel()

	config := `iam_assume_role_session_name = "terragrunt-iam-assume-role-session-name"`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, "terragrunt-iam-assume-role-session-name", terragruntConfig.IamAssumeRoleSessionName)
}

func TestParseTerragruntConfigDependenciesOnePath(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
	paths = ["../test/fixture-parent-folders/multiple-terragrunt-in-parents"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture-parent-folders/multiple-terragrunt-in-parents"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigDependenciesMultiplePaths(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
	paths = ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	source = "foo"
}

remote_state {
	backend = "s3"
	config = {
		encrypt = true
		bucket = "my-bucket"
		key = "terraform.tfstate"
		region = "us-east-1"
	}
}

dependencies {
	paths = ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntJsonConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"terraform": {
		"source": "foo"
	},
	"remote_state": {
		"backend": "s3",
		"config": {
			"encrypt": true,
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region": "us-east-1"
		}
	},
	"dependencies":{
		"paths": ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigInclude(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
include {
	path = "../../../%s"
}
`, DefaultTerragruntConfigPath)

	opts := options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath,
		NonInteractive:       true,
		Logger:               util.CreateLogEntry("", util.GetDefaultLogLevel()),
	}

	terragruntConfig, err := ParseConfigString(config, &opts, nil, opts.TerragruntConfigPath, nil)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeWithFindInParentFolders(t *testing.T) {
	t.Parallel()

	config := `
include {
	path = find_in_parent_folders()
}
`

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeOverrideRemote(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
include {
	path = "../../../%s"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
	backend = "s3"
	config = {
		encrypt = false
		bucket = "override"
		key = "override"
		region = "override"
	}
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, false, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
include {
	path = "../../../%s"
}

terraform {
	source = "foo"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
	backend = "s3"
	config = {
		encrypt = false
		bucket = "override"
		key = "override"
		region = "override"
	}
}

dependencies {
	paths = ["override"]
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err))

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, false, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["region"])
	}

	assert.Equal(t, []string{"override"}, terragruntConfig.Dependencies.Paths)
}

func TestParseTerragruntJsonConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
{
	"include":{
		"path": "../../../%s"
	},
	"terraform":{
		"source": "foo"
	},
	"remote_state":{
		"backend": "s3",
		"config":{
			"encrypt": false,
			"bucket": "override",
			"key": "override",
			"region": "override"
		}
	},
	"dependencies":{
		"paths": ["override"]
	}
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntJsonConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err))

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, false, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.Config["region"])
	}

	assert.Equal(t, []string{"override"}, terragruntConfig.Dependencies.Paths)
}

func TestParseTerragruntConfigTwoLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/" + DefaultTerragruntConfigPath

	config, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)

	_, actualErr := ParseConfigString(config, opts, nil, configPath, nil)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
		SecondLevelIncludePath: absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
	}
	assert.True(t, errors.IsError(actualErr, expectedErr), "Expected error %v but got %v", expectedErr, actualErr)
}

func TestParseTerragruntConfigThreeLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath

	config, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)

	_, actualErr := ParseConfigString(config, opts, nil, configPath, nil)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
		SecondLevelIncludePath: absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
	}
	assert.True(t, errors.IsError(actualErr, expectedErr), "Expected error %v but got %v", expectedErr, actualErr)
}

func TestParseTerragruntConfigEmptyConfig(t *testing.T) {
	t.Parallel()

	config := ``

	cfg, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	assert.NoError(t, err)

	assert.Nil(t, cfg.Terraform)
	assert.Nil(t, cfg.RemoteState)
	assert.Nil(t, cfg.Dependencies)

	assert.Empty(t, cfg.TerraformBinary)
	assert.Nil(t, cfg.RetryMaxAttempts)
	assert.Nil(t, cfg.RetrySleepIntervalSec)
	assert.Nil(t, cfg.RetryableErrors)
	assert.Empty(t, cfg.DownloadDir)
	assert.Empty(t, cfg.TerraformVersionConstraint)
	assert.Empty(t, cfg.TerragruntVersionConstraint)
	assert.Nil(t, cfg.PreventDestroy)
	assert.False(t, cfg.Skip)
	assert.Empty(t, cfg.IamRole)
	assert.Empty(t, cfg.IamAssumeRoleDuration)
	assert.Empty(t, cfg.IamAssumeRoleSessionName)
}

func TestParseTerragruntConfigEmptyConfigOldConfig(t *testing.T) {
	t.Parallel()

	config := ``

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
}

func TestParseTerragruntConfigTerraformNoSource(t *testing.T) {
	t.Parallel()

	config := `
terraform {}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	require.NotNil(t, terragruntConfig.Terraform)
	require.Nil(t, terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithSource(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	source = "foo"
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	extra_arguments "secrets" {
		arguments = [
			"-var-file=terraform.tfvars",
			"-var-file=terraform-secret.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
		env_vars = {
			TEST_VAR = "value"
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "secrets", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t,
			&[]string{
				"-var-file=terraform.tfvars",
				"-var-file=terraform-secret.tfvars",
			},
			terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t,
			TERRAFORM_COMMANDS_NEED_VARS,
			terragruntConfig.Terraform.ExtraArgs[0].Commands)

		assert.Equal(t,
			&map[string]string{"TEST_VAR": "value"},
			terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	}
}

func TestParseTerragruntConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	extra_arguments "json_output" {
		arguments = ["-json"]
		commands = ["output"]
	}

	extra_arguments "fmt_diff" {
		arguments = ["-diff=true"]
		commands = ["fmt"]
	}

	extra_arguments "required_tfvars" {
		required_var_files = [
			"file1.tfvars",
			"file2.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
	}

	extra_arguments "optional_tfvars" {
		optional_var_files = [
			"opt1.tfvars",
			"opt2.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "json_output", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t, &[]string{"-json"}, terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t, []string{"output"}, terragruntConfig.Terraform.ExtraArgs[0].Commands)
		assert.Equal(t, "fmt_diff", terragruntConfig.Terraform.ExtraArgs[1].Name)
		assert.Equal(t, &[]string{"-diff=true"}, terragruntConfig.Terraform.ExtraArgs[1].Arguments)
		assert.Equal(t, []string{"fmt"}, terragruntConfig.Terraform.ExtraArgs[1].Commands)
		assert.Equal(t, "required_tfvars", terragruntConfig.Terraform.ExtraArgs[2].Name)
		assert.Equal(t, &[]string{"file1.tfvars", "file2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[2].RequiredVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

func TestParseTerragruntJsonConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
{
	"terraform":{
		"extra_arguments":{
			"json_output":{
				"arguments": ["-json"],
				"commands": ["output"]
			},
			"fmt_diff":{
				"arguments": ["-diff=true"],
				"commands": ["fmt"]
			},
			"required_tfvars":{
				"required_var_files":[
					"file1.tfvars",
					"file2.tfvars"
				],
				"commands": "${get_terraform_commands_that_need_vars()}"
			},
			"optional_tfvars":{
				"optional_var_files":[
					"opt1.tfvars",
					"opt2.tfvars"
				],
				"commands": "${get_terraform_commands_that_need_vars()}"
			}
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "json_output", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t, &[]string{"-json"}, terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t, []string{"output"}, terragruntConfig.Terraform.ExtraArgs[0].Commands)
		assert.Equal(t, "fmt_diff", terragruntConfig.Terraform.ExtraArgs[1].Name)
		assert.Equal(t, &[]string{"-diff=true"}, terragruntConfig.Terraform.ExtraArgs[1].Arguments)
		assert.Equal(t, []string{"fmt"}, terragruntConfig.Terraform.ExtraArgs[1].Commands)
		assert.Equal(t, "required_tfvars", terragruntConfig.Terraform.ExtraArgs[2].Name)
		assert.Equal(t, &[]string{"file1.tfvars", "file2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[2].RequiredVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

// TODO: Test throws a SegFault because of derefenrencing a nil pointer
//
// func TestSetIAMRole(t *testing.T) {
// 	t.Parallel()

// 	testConfigString := ""
// 	testTerragruntOptions := options.TerragruntOptions{}
// 	testIncludeFromChild := IncludeConfig{}
// 	testFilename := "locals.hcl"

// 	err := setIAMRole(testConfigString, &testTerragruntOptions, &testIncludeFromChild, testFilename)

// 	assert.Nil(t, err)
// }

func TestDecodeAsTerragruntConfigFileError(t *testing.T) {
	t.Parallel()

	testFile := hcl.File{}
	testFilename := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testExtensions := EvalContextExtensions{}

	actualResult, err := decodeAsTerragruntConfigFile(&testFile, testFilename, &testTerragruntOptions, testExtensions)

	assert.Nil(t, actualResult)
	assert.Error(t, err)
}

func TestDecodeAsTerragruntConfigFile(t *testing.T) {
	t.Parallel()

	mockBody := MockHclBody{}

	testFile := hcl.File{
		Body: mockBody,
	}
	testFilename := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testExtensions := EvalContextExtensions{}

	actualResult, err := decodeAsTerragruntConfigFile(&testFile, testFilename, &testTerragruntOptions, testExtensions)

	expectedResult := terragruntConfigFile{}

	assert.Equal(t, &expectedResult, actualResult)
	assert.NoError(t, err)
}

func TestGetIndexOfHookWith(t *testing.T) {
	t.Parallel()

	dummyHook := Hook{
		Name: "HitMe",
	}
	testHook := []Hook{dummyHook}
	testName := "HitMe"

	actualResult := getIndexOfHookWithName(testHook, testName)

	assert.Equal(t, 0, actualResult)
}

func TestGetIndexOfHookWithNameNoElement(t *testing.T) {
	t.Parallel()

	testHook := []Hook{}
	testName := "NotThere"

	actualResult := getIndexOfHookWithName(testHook, testName)

	assert.Equal(t, -1, actualResult)
}

func TestGetIndexOfErrorHookWithName(t *testing.T) {
	t.Parallel()

	dummyErrorHook := ErrorHook{
		Name: "HitMe",
	}
	testErrorHook := []ErrorHook{dummyErrorHook}
	testName := "HitMe"

	actualResult := getIndexOfErrorHookWithName(testErrorHook, testName)

	assert.Equal(t, 0, actualResult)
}

func TestGetIndexOfErrorHookWithNameNoElement(t *testing.T) {
	t.Parallel()

	testErrorHook := []ErrorHook{}
	testName := "NotThere"

	actualResult := getIndexOfErrorHookWithName(testErrorHook, testName)

	assert.Equal(t, -1, actualResult)
}

func TestGetIndexOfExtraArgsWithName(t *testing.T) {
	t.Parallel()

	dummyTerraformExtraArguments := TerraformExtraArguments{
		Name: "HitMe",
	}
	testTerraformExtraArguments := []TerraformExtraArguments{dummyTerraformExtraArguments}
	testName := "HitMe"

	actualResult := getIndexOfExtraArgsWithName(testTerraformExtraArguments, testName)

	assert.Equal(t, 0, actualResult)
}

func TestGetIndexOfExtraArgsWithNameNoElement(t *testing.T) {
	t.Parallel()

	testTerraformExtraArguments := []TerraformExtraArguments{}
	testName := "NotThere"

	actualResult := getIndexOfExtraArgsWithName(testTerraformExtraArguments, testName)

	assert.Equal(t, -1, actualResult)
}

func TestConvertToTerragruntConfigRemoteStateFailsParsing(t *testing.T) {
	t.Parallel()

	dummyRemoteStateConfigFile := remoteStateConfigFile{
		Config: cty.StringVal("breakMe"),
	}

	testTerragruntConfigFromFile := terragruntConfigFile{
		RemoteState: &dummyRemoteStateConfigFile,
	}
	testConfigPath := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testContextExtensions := EvalContextExtensions{}

	actualResult, err := convertToTerragruntConfig(&testTerragruntConfigFromFile, testConfigPath, &testTerragruntOptions, testContextExtensions)

	assert.Nil(t, actualResult)
	assert.Error(t, err)
}

func TestConvertToTerragruntConfigRemoteStateAttrFailsParsing(t *testing.T) {
	t.Parallel()

	dummyRemoteStateAttr := cty.StringVal("BreakMe")

	testTerragruntConfigFromFile := terragruntConfigFile{
		RemoteStateAttr: &dummyRemoteStateAttr,
	}
	testConfigPath := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testContextExtensions := EvalContextExtensions{}

	actualResult, err := convertToTerragruntConfig(&testTerragruntConfigFromFile, testConfigPath, &testTerragruntOptions, testContextExtensions)

	assert.Nil(t, actualResult)
	assert.Error(t, err)
}

func TestConvertToTerragruntConfigHooksFailValidation(t *testing.T) {
	t.Parallel()

	testBackend := "my-backend"

	dummyCtyMap := make(map[string]cty.Value)
	dummyCtyMap["Backend"] = cty.StringVal(testBackend)
	stubCtyValue := cty.MapVal(dummyCtyMap)

	dummyRemoteStateConfigFile := remoteStateConfigFile{
		Backend: testBackend,
		Config:  stubCtyValue,
	}
	dummyRemoteStateAttr := stubCtyValue

	stubHook := Hook{}

	testBeforeHooks := make([]Hook, 1)
	testBeforeHooks = append(testBeforeHooks, stubHook)
	dummyTerraformConfig := TerraformConfig{
		BeforeHooks: testBeforeHooks,
	}

	testTerragruntConfigFromFile := terragruntConfigFile{
		RemoteState:     &dummyRemoteStateConfigFile,
		RemoteStateAttr: &dummyRemoteStateAttr,
		Terraform:       &dummyTerraformConfig,
	}
	testConfigPath := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testContextExtensions := EvalContextExtensions{}

	actualResult, err := convertToTerragruntConfig(&testTerragruntConfigFromFile, testConfigPath, &testTerragruntOptions, testContextExtensions)

	assert.Nil(t, actualResult)
	assert.Error(t, err)
}

// TODO: Add more error testing tests above
func TestConvertToTerragruntEmptyConfigPasses(t *testing.T) {
	t.Parallel()

	testTerragruntConfigFromFile := terragruntConfigFile{}
	testConfigPath := ""
	testTerragruntOptions := options.TerragruntOptions{}
	testContextExtensions := EvalContextExtensions{}

	actualResult, err := convertToTerragruntConfig(&testTerragruntConfigFromFile, testConfigPath, &testTerragruntOptions, testContextExtensions)

	assert.NotNil(t, actualResult)
	assert.NoError(t, err)
}

func TestFindConfigFilesInPathNone(t *testing.T) {
	t.Parallel()

	expected := []string{}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/none", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-config/subdir/terragrunt.hcl"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneJsonConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-json-config/subdir/terragrunt.hcl.json"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-json-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleJsonConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-json-configs/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-json-configs/subdir-2/subdir/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-json-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-json-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleMixedConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-mixed-configs/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-mixed-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixture-config-files/multiple-mixed-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-mixed-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerragruntCache(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-cached-config/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-cached-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnv(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = ".tf_data"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvPath(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = "subdir/.tf_data"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvRoot(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	expected := []string{
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl"),
	}
	workingDir := filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/")
	terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = filepath.Join(workingDir, ".tf_data")

	actual, err := FindConfigFilesInPath(workingDir, terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresDownloadDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.DownloadDir = "../test/fixture-config-files/multiple-configs/subdir-2"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func mockOptionsForTestWithConfigPath(t *testing.T, configPath string) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Failed to create TerragruntOptions: %v", err)
	}
	return opts
}

func mockOptionsForTest(t *testing.T) *options.TerragruntOptions {
	return mockOptionsForTestWithConfigPath(t, "test-time-mock")
}

func TestParseTerragruntConfigPreventDestroyTrue(t *testing.T) {
	t.Parallel()

	config := `
prevent_destroy = true
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, true, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigPreventDestroyFalse(t *testing.T) {
	t.Parallel()

	config := `
prevent_destroy = false
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, false, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigSkipTrue(t *testing.T) {
	t.Parallel()

	config := `
skip = true
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, true, terragruntConfig.Skip)
}

func TestParseTerragruntConfigSkipFalse(t *testing.T) {
	t.Parallel()

	config := `
skip = false
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, false, terragruntConfig.Skip)
}

func TestIncludeFunctionsWorkInChildConfig(t *testing.T) {
	config := `
include {
	path = find_in_parent_folders()
}
terraform {
	source = path_relative_to_include()
}
`
	opts := options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/" + DefaultTerragruntConfigPath,
		NonInteractive:       true,
		MaxFoldersToCheck:    5,
		Logger:               util.CreateLogEntry("", util.GetDefaultLogLevel()),
	}

	terragruntConfig, err := ParseConfigString(config, &opts, nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "child", *terragruntConfig.Terraform.Source)
}

func TestModuleDependenciesMerge(t *testing.T) {
	testCases := []struct {
		name     string
		target   []string
		source   []string
		expected []string
	}{
		{
			"MergeNil",
			[]string{"../vpc", "../sql"},
			nil,
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeOne",
			[]string{"../vpc", "../sql"},
			[]string{"../services"},
			[]string{"../vpc", "../sql", "../services"},
		},
		{
			"MergeMany",
			[]string{"../vpc", "../sql"},
			[]string{"../services", "../groups"},
			[]string{"../vpc", "../sql", "../services", "../groups"},
		},
		{
			"MergeEmpty",
			[]string{"../vpc", "../sql"},
			[]string{},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeOneExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc"},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeAllExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeSomeExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../services"},
			[]string{"../vpc", "../sql", "../services"},
		},
	}

	for _, testCase := range testCases {
		// Capture range variable so that it is brought into the scope within the for loop, so that it is stable even
		// when subtests are run in parallel.
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			target := &ModuleDependencies{Paths: testCase.target}

			var source *ModuleDependencies = nil
			if testCase.source != nil {
				source = &ModuleDependencies{Paths: testCase.source}
			}

			target.Merge(source)
			assert.Equal(t, target.Paths, testCase.expected)
		})
	}
}

func ptr(str string) *string {
	return &str
}
