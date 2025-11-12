//go:build docker && tofu

package test_test

import (
	"crypto/rand"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testFixtureTofuStateEncryptionOpenbao = "fixtures/tofu-state-encryption/openbao"
)

func setupOpenbao(t *testing.T) (baoC *testcontainers.DockerContainer, baoToken string, baoAddr string) {
	t.Helper()

	baoToken = rand.Text()

	baoC, baoAddr = helpers.RunContainer(t, "openbao/openbao:2.4.1", 8200,
		testcontainers.WithWaitStrategy(
			wait.ForLog("core: vault is unsealed"),
		),
		testcontainers.WithEnv(map[string]string{
			"BAO_DEV_ROOT_TOKEN_ID": baoToken,
		}),
	)

	execOptions := []tcexec.ProcessOption{
		tcexec.WithEnv([]string{"BAO_ADDR=http://localhost:8200", "VAULT_TOKEN=" + baoToken}),
	}

	helpers.ContExecNoOutput(t, baoC, "bao secrets enable transit", execOptions...)

	return baoC, baoToken, baoAddr
}

func TestTofuStateEncryptionOpenbao(t *testing.T) {
	t.Parallel()

	baoC, baoToken, baoAddr := setupOpenbao(t)
	baoKey := rand.Text()
	baoKeyPath := "transit/keys/" + baoKey

	execOptions := []tcexec.ProcessOption{
		tcexec.WithEnv([]string{"BAO_ADDR=http://localhost:8200", "VAULT_TOKEN=" + baoToken}),
	}

	helpers.ContExecNoOutput(t, baoC, "bao write -f "+baoKeyPath, execOptions...)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuStateEncryptionOpenbao)
	workDir := util.JoinPath(tmpEnvPath, testFixtureTofuStateEncryptionOpenbao)
	configPath := util.JoinPath(workDir, "terragrunt.hcl")

	helpers.CopyAndFillMapPlaceholders(t, configPath, configPath, map[string]string{
		"__FILL_IN_OPENBAO_KEY_NAME__": baoKey,
		"__FILL_IN_OPENBAO_ADDRESS__":  baoAddr,
		"__FILL_IN_OPENBAO_TOKEN__":    baoToken,
	})

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+workDir)
	assert.True(t, helpers.FileIsInFolder(t, stateFile, workDir))
	validateStateIsEncrypted(t, stateFile, workDir)
}
