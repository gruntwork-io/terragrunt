package locks

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestGetIpAddress(t *testing.T) {
	t.Parallel()

	ip, err := getIpAddress()

	assert.Nil(t, err)
	assertIsValidIp(t, ip)
}

func TestCreateLockMetadata(t *testing.T) {
	t.Parallel()

	expectedStateFileId := "expected-state-file-id"
	lockMetadata, err := CreateLockMetadata(expectedStateFileId)

	assert.Nil(t, err)
	assert.Equal(t, expectedStateFileId, lockMetadata.StateFileId)
	assert.False(t, lockMetadata.DateCreated.IsZero())
	assertIsValidIp(t, lockMetadata.IpAddress)
	assert.NotEmpty(t, lockMetadata.Username)
}

func assertIsValidIp(t *testing.T, ip string) {
	assert.Regexp(t, "\\d+.\\d+.\\d+.\\d+", ip)
}